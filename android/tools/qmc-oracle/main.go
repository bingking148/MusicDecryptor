package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"

	"golang.org/x/crypto/tea"
)

func simpleMakeKey(salt byte, length int) []byte {
	keyBuf := make([]byte, length)
	for i := 0; i < length; i++ {
		tmp := math.Tan(float64(salt) + float64(i)*0.1)
		keyBuf[i] = byte(math.Abs(tmp) * 100.0)
	}
	return keyBuf
}

func xor8(dst, a, b []byte) {
	for i := 0; i < 8; i++ {
		dst[i] = a[i] ^ b[i]
	}
}

func decryptTencentTea(inBuf []byte, key []byte) ([]byte, error) {
	const saltLen = 2
	const zeroLen = 7
	if len(inBuf)%8 != 0 || len(inBuf) < 16 {
		return nil, fmt.Errorf("bad length")
	}
	blk, err := tea.NewCipherWithRounds(key, 32)
	if err != nil {
		return nil, err
	}
	dest := make([]byte, 8)
	blk.Decrypt(dest, inBuf[:8])
	padLen := int(dest[0] & 0x7)
	outLen := len(inBuf) - 1 - padLen - saltLen - zeroLen
	if outLen < 0 {
		return nil, fmt.Errorf("negative out")
	}
	out := make([]byte, outLen)
	ivPrev := make([]byte, 8)
	inPos := 8
	destIdx := 1 + padLen

	cryptBlock := func() error {
		if inPos+8 > len(inBuf) {
			return fmt.Errorf("truncated")
		}
		copy(ivPrev, inBuf[inPos-8:inPos])
		next := inBuf[inPos : inPos+8]
		xor8(dest, dest, next)
		blk.Decrypt(dest, dest)
		inPos += 8
		destIdx = 0
		return nil
	}
	for i := 1; i <= saltLen; {
		if destIdx < 8 {
			destIdx++
			i++
		} else if err := cryptBlock(); err != nil {
			return nil, err
		}
	}
	for n := 0; n < outLen; {
		if destIdx < 8 {
			out[n] = dest[destIdx] ^ ivPrev[destIdx]
			destIdx++
			n++
		} else if err := cryptBlock(); err != nil {
			return nil, err
		}
	}
	for i := 1; i <= zeroLen; {
		if destIdx < 8 {
			if dest[destIdx] != ivPrev[destIdx] {
				return nil, fmt.Errorf("zero check failed")
			}
			destIdx++
			i++
		} else if err := cryptBlock(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Deterministic stand-in for rand() so vectors stay stable.
type rng struct{ n byte }

func (r *rng) next() byte {
	r.n++
	return 0xA0 + r.n
}

func encryptTencentTea(body, key []byte) ([]byte, error) {
	const saltLen = 2
	const zeroLen = 7
	blk, err := tea.NewCipherWithRounds(key, 32)
	if err != nil {
		return nil, err
	}
	nPadSaltBodyZero := len(body) + 1 + saltLen + zeroLen
	padLen := nPadSaltBodyZero % 8
	if padLen != 0 {
		padLen = 8 - padLen
	}
	r := rng{}
	src := [8]byte{}
	src[0] = (r.next() & 0xF8) | byte(padLen)
	srcI := 1
	for n := padLen; n > 0; n-- {
		src[srcI] = r.next()
		srcI++
	}
	ivPlain := make([]byte, 8)
	ivCrypt := make([]byte, 8) // zeros
	var out []byte

	flush := func() {
		block := make([]byte, 8)
		copy(block, src[:])
		for j := 0; j < 8; j++ {
			block[j] ^= ivCrypt[j]
		}
		dst := make([]byte, 8)
		blk.Encrypt(dst, block)
		for j := 0; j < 8; j++ {
			dst[j] ^= ivPlain[j]
		}
		copy(ivPlain, block)
		copy(ivCrypt, dst)
		out = append(out, dst...)
		src = [8]byte{}
		srcI = 0
	}

	for i := 1; i <= saltLen; {
		if srcI < 8 {
			src[srcI] = r.next()
			srcI++
			i++
		}
		if srcI == 8 {
			flush()
		}
	}
	bodyPos := 0
	for bodyPos < len(body) {
		if srcI < 8 {
			src[srcI] = body[bodyPos]
			srcI++
			bodyPos++
		}
		if srcI == 8 {
			flush()
		}
	}
	for i := 1; i <= zeroLen; {
		if srcI < 8 {
			src[srcI] = 0
			srcI++
			i++
		}
		if srcI == 8 {
			flush()
		}
	}
	return out, nil
}

func deriveKeyV1(raw []byte) ([]byte, error) {
	if len(raw) < 16 {
		return nil, fmt.Errorf("short")
	}
	simple := simpleMakeKey(106, 8)
	teaKey := make([]byte, 16)
	for i := 0; i < 8; i++ {
		teaKey[i<<1] = simple[i]
		teaKey[i<<1+1] = raw[i]
	}
	body, err := decryptTencentTea(raw[8:], teaKey)
	if err != nil {
		return nil, err
	}
	return append(append([]byte{}, raw[:8]...), body...), nil
}

var v2k1 = []byte{0x33, 0x38, 0x36, 0x5A, 0x4A, 0x59, 0x21, 0x40, 0x23, 0x2A, 0x24, 0x25, 0x5E, 0x26, 0x29, 0x28}
var v2k2 = []byte{0x2A, 0x2A, 0x23, 0x21, 0x28, 0x23, 0x24, 0x25, 0x26, 0x5E, 0x61, 0x31, 0x63, 0x5A, 0x2C, 0x54}

func deriveKey(ekey []byte) ([]byte, error) {
	dec := make([]byte, base64.StdEncoding.DecodedLen(len(ekey)))
	n, err := base64.StdEncoding.Decode(dec, ekey)
	if err != nil {
		return nil, err
	}
	dec = dec[:n]
	const prefix = "QQMusic EncV2,Key:"
	if bytes.HasPrefix(dec, []byte(prefix)) {
		rest := dec[len(prefix):]
		b, err := decryptTencentTea(rest, v2k1)
		if err != nil {
			return nil, err
		}
		b, err = decryptTencentTea(b, v2k2)
		if err != nil {
			return nil, err
		}
		b = bytes.TrimRight(b, "\x00")
		raw := make([]byte, base64.StdEncoding.DecodedLen(len(b)))
		n, err := base64.StdEncoding.Decode(raw, b)
		if err != nil {
			return nil, err
		}
		dec = raw[:n]
	}
	return deriveKeyV1(dec)
}

func makeV1(keyStream []byte) (string, error) {
	header := keyStream[:8]
	body := keyStream[8:]
	simple := simpleMakeKey(106, 8)
	teaKey := make([]byte, 16)
	for i := 0; i < 8; i++ {
		teaKey[i<<1] = simple[i]
		teaKey[i<<1+1] = header[i]
	}
	enc, err := encryptTencentTea(body, teaKey)
	if err != nil {
		return "", err
	}
	raw := append(append([]byte{}, header...), enc...)
	return base64.StdEncoding.EncodeToString(raw), nil
}

func rotate(value, bits byte) byte {
	rot := (bits + 4) % 8
	return (value << rot) | (value >> rot)
}

func mapDecrypt(key, buf []byte, offset int) {
	n := len(key)
	for i := range buf {
		off := offset + i
		if off > 0x7FFF {
			off %= 0x7FFF
		}
		idx := (off*off + 71214) % n
		buf[i] ^= rotate(key[idx], byte(idx)&7)
	}
}

func newRC4(key []byte) (box []byte, hash uint32) {
	n := len(key)
	box = make([]byte, n)
	for i := 0; i < n; i++ {
		box[i] = byte(i)
	}
	j := 0
	for i := 0; i < n; i++ {
		j = (j + int(box[i]) + int(key[i%n])) % n
		box[i], box[j] = box[j], box[i]
	}
	hash = 1
	for i := 0; i < n; i++ {
		v := uint32(key[i])
		if v == 0 {
			continue
		}
		next := hash * v
		if next == 0 || next <= hash {
			break
		}
		hash = next
	}
	return box, hash
}

func segmentSkip(key []byte, hash uint32, id int) int {
	n := len(key)
	seed := int(key[id%n])
	if seed == 0 {
		return 0
	}
	idx := int64(float64(hash) / float64((id+1)*seed) * 100.0)
	return int(idx % int64(n))
}

func rc4Decrypt(key, box []byte, hash uint32, buf []byte, offset int) {
	const seg = 5120
	const first = 128
	n := len(key)
	toProcess := len(buf)
	processed := 0
	mark := func(p int) bool {
		offset += p
		toProcess -= p
		processed += p
		return toProcess == 0
	}
	if offset < first {
		block := toProcess
		if block > first-offset {
			block = first - offset
		}
		for i := 0; i < block; i++ {
			buf[i] ^= key[segmentSkip(key, hash, offset+i)]
		}
		if mark(block) {
			return
		}
	}
	encSeg := func(dst []byte, off int) {
		b := make([]byte, n)
		copy(b, box)
		skip := (off % seg) + segmentSkip(key, hash, off/seg)
		j, k := 0, 0
		for i := -skip; i < len(dst); i++ {
			j = (j + 1) % n
			k = (int(b[j]) + k) % n
			b[j], b[k] = b[k], b[j]
			if i >= 0 {
				dst[i] ^= b[(int(b[j])+int(b[k]))%n]
			}
		}
	}
	if offset%seg != 0 {
		block := toProcess
		if block > seg-offset%seg {
			block = seg - offset%seg
		}
		encSeg(buf[processed:processed+block], offset)
		if mark(block) {
			return
		}
	}
	for toProcess > seg {
		encSeg(buf[processed:processed+seg], offset)
		mark(seg)
	}
	if toProcess > 0 {
		encSeg(buf[processed:], offset)
	}
}

var staticBox = []byte{
	0x77, 0x48, 0x32, 0x73, 0xDE, 0xF2, 0xC0, 0xC8,
	0x95, 0xEC, 0x30, 0xB2, 0x51, 0xC3, 0xE1, 0xA0,
	0x9E, 0xE6, 0x9D, 0xCF, 0xFA, 0x7F, 0x14, 0xD1,
	0xCE, 0xB8, 0xDC, 0xC3, 0x4A, 0x67, 0x93, 0xD6,
	0x28, 0xC2, 0x91, 0x70, 0xCA, 0x8D, 0xA2, 0xA4,
	0xF0, 0x08, 0x61, 0x90, 0x7E, 0x6F, 0xA2, 0xE0,
	0xEB, 0xAE, 0x3E, 0xB6, 0x67, 0xC7, 0x92, 0xF4,
	0x91, 0xB5, 0xF6, 0x6C, 0x5E, 0x84, 0x40, 0xF7,
	0xF3, 0x1B, 0x02, 0x7F, 0xD5, 0xAB, 0x41, 0x89,
	0x28, 0xF4, 0x25, 0xCC, 0x52, 0x11, 0xAD, 0x43,
	0x68, 0xA6, 0x41, 0x8B, 0x84, 0xB5, 0xFF, 0x2C,
	0x92, 0x4A, 0x26, 0xD8, 0x47, 0x6A, 0x7C, 0x95,
	0x61, 0xCC, 0xE6, 0xCB, 0xBB, 0x3F, 0x47, 0x58,
	0x89, 0x75, 0xC3, 0x75, 0xA1, 0xD9, 0xAF, 0xCC,
	0x08, 0x73, 0x17, 0xDC, 0xAA, 0x9A, 0xA2, 0x16,
	0x41, 0xD8, 0xA2, 0x06, 0xC6, 0x8B, 0xFC, 0x66,
	0x34, 0x9F, 0xCF, 0x18, 0x23, 0xA0, 0x0A, 0x74,
	0xE7, 0x2B, 0x27, 0x70, 0x92, 0xE9, 0xAF, 0x37,
	0xE6, 0x8C, 0xA7, 0xBC, 0x62, 0x65, 0x9C, 0xC2,
	0x08, 0xC9, 0x88, 0xB3, 0xF3, 0x43, 0xAC, 0x74,
	0x2C, 0x0F, 0xD4, 0xAF, 0xA1, 0xC3, 0x01, 0x64,
	0x95, 0x4E, 0x48, 0x9F, 0xF4, 0x35, 0x78, 0x95,
	0x7A, 0x39, 0xD6, 0x6A, 0xA0, 0x6D, 0x40, 0xE8,
	0x4F, 0xA8, 0xEF, 0x11, 0x1D, 0xF3, 0x1B, 0x3F,
	0x3F, 0x07, 0xDD, 0x6F, 0x5B, 0x19, 0x30, 0x19,
	0xFB, 0xEF, 0x0E, 0x37, 0xF0, 0x0E, 0xCD, 0x16,
	0x49, 0xFE, 0x53, 0x47, 0x13, 0x1A, 0xBD, 0xA4,
	0xF1, 0x40, 0x19, 0x60, 0x0E, 0xED, 0x68, 0x09,
	0x06, 0x5F, 0x4D, 0xCF, 0x3D, 0x1A, 0xFE, 0x20,
	0x77, 0xE4, 0xD9, 0xDA, 0xF9, 0xA4, 0x2B, 0x76,
	0x1C, 0x71, 0xDB, 0x00, 0xBC, 0xFD, 0x0C, 0x6C,
	0xA5, 0x47, 0xF7, 0xF6, 0x00, 0x79, 0x4A, 0x11,
}

func staticDecrypt(buf []byte, offset int) {
	for i := range buf {
		off := offset + i
		if off > 0x7FFF {
			off %= 0x7FFF
		}
		idx := (off*off + 27) & 0xFF
		buf[i] ^= staticBox[idx]
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	for _, n := range []int{0, 1, 2, 7, 8, 15, 16, 31, 32, 100, 300} {
		msg := bytes.Repeat([]byte("music-decryptor-body"), 1+n/10)
		msg = msg[:n]
		key := []byte("0123456789abcdef")
		enc, err := encryptTencentTea(msg, key)
		must(err)
		dec, err := decryptTencentTea(enc, key)
		must(err)
		if !bytes.Equal(dec, msg) {
			panic(fmt.Sprintf("tea roundtrip %d: %q vs %q", n, dec, msg))
		}
	}

	simple := simpleMakeKey(106, 8)

	blk, err := tea.NewCipherWithRounds([]byte("0123456789abcdef"), 32)
	must(err)
	teaSrc := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	teaDst := make([]byte, 8)
	blk.Encrypt(teaDst, teaSrc)
	teaBack := make([]byte, 8)
	blk.Decrypt(teaBack, teaDst)
	if !bytes.Equal(teaBack, teaSrc) {
		panic("tea block")
	}

	// Map key, 32 bytes, no zero to keep hash interesting. First 8 are the v1 header.
	mapKey := make([]byte, 32)
	for i := range mapKey {
		mapKey[i] = byte(0x31 + (i*17)%180)
	}
	mapEkey, err := makeV1(mapKey)
	must(err)
	got, err := deriveKey([]byte(mapEkey))
	must(err)
	if !bytes.Equal(got, mapKey) {
		panic(fmt.Sprintf("derive map\n got %x\nwant %x", got, mapKey))
	}

	rc4Key := make([]byte, 320)
	for i := range rc4Key {
		rc4Key[i] = byte(1 + (i*13)%251)
	}
	rc4Ekey, err := makeV1(rc4Key)
	must(err)
	got, err = deriveKey([]byte(rc4Ekey))
	must(err)
	if !bytes.Equal(got, rc4Key) {
		panic("derive rc4")
	}

	// v2 wraps the v1 raw (not the decoded key).
	header := mapKey[:8]
	body := mapKey[8:]
	teaKey := make([]byte, 16)
	for i := 0; i < 8; i++ {
		teaKey[i<<1] = simple[i]
		teaKey[i<<1+1] = header[i]
	}
	encBody, err := encryptTencentTea(body, teaKey)
	must(err)
	v1raw := append(append([]byte{}, header...), encBody...)
	v1b64 := []byte(base64.StdEncoding.EncodeToString(v1raw))
	ct2, err := encryptTencentTea(v1b64, v2k2)
	must(err)
	ct1, err := encryptTencentTea(ct2, v2k1)
	must(err)
	v2plain := append([]byte("QQMusic EncV2,Key:"), ct1...)
	v2ekey := base64.StdEncoding.EncodeToString(v2plain)
	got, err = deriveKey([]byte(v2ekey))
	must(err)
	if !bytes.Equal(got, mapKey) {
		panic(fmt.Sprintf("derive v2\n got %x\nwant %x", got, mapKey))
	}

	audio := []byte("fLaC")
	audio = append(audio, bytes.Repeat([]byte{0x10, 0x20, 0x30, 0x40}, 80)...)
	sealed := append([]byte{}, audio...)
	mapDecrypt(mapKey, sealed, 0) // self-inverse
	ekeyBytes := []byte(mapEkey)
	file := append(append([]byte{}, sealed...), ekeyBytes...)
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(ekeyBytes)))
	file = append(file, lenBuf[:]...)

	// QTag variant
	qPayload := mapEkey + ",42,7"
	qFile := append(append([]byte{}, sealed...), []byte(qPayload)...)
	var be [4]byte
	binary.BigEndian.PutUint32(be[:], uint32(len(qPayload)))
	qFile = append(qFile, be[:]...)
	qFile = append(qFile, []byte("QTag")...)

	// STag variant
	stagPayload := "12345,2,AbC123mid"
	sFile := append(append([]byte{}, sealed...), []byte(stagPayload)...)
	binary.BigEndian.PutUint32(be[:], uint32(len(stagPayload)))
	sFile = append(sFile, be[:]...)
	sFile = append(sFile, []byte("STag")...)

	// Static: encrypt header and force tail not to look like a key length.
	staticPlain := append([]byte("fLaC-static-test-vector!!"), bytes.Repeat([]byte{0x5A}, 40)...)
	staticCipher := append([]byte{}, staticPlain...)
	staticDecrypt(staticCipher, 0)
	for {
		tail := binary.LittleEndian.Uint32(staticCipher[len(staticCipher)-4:])
		if tail == 0 || tail > 0xFFFF {
			break
		}
		staticPlain = append(staticPlain, 0x11)
		staticCipher = append([]byte{}, staticPlain...)
		staticDecrypt(staticCipher, 0)
	}

	plainRC4 := bytes.Repeat([]byte("OggS-rc4-segment-check"), 300) // 6300 > 5120
	box, hash := newRC4(rc4Key)
	rc4Out := append([]byte{}, plainRC4...)
	rc4Decrypt(rc4Key, box, hash, rc4Out, 0)
	// second window at an unaligned offset
	rc4Win := append([]byte{}, plainRC4[100:5300]...)
	rc4Decrypt(rc4Key, box, hash, rc4Win, 100)

	mapWin := append([]byte{}, audio...)
	mapDecrypt(mapKey, mapWin, 0)
	mapOff := append([]byte{}, audio...)
	mapDecrypt(mapKey, mapOff, 9000)

	plainSum := sha256.Sum256(plainRC4)
	sum := sha256.Sum256(rc4Out)

	vec := map[string]any{
		"simpleKey":        hex.EncodeToString(simple),
		"teaKey":           hex.EncodeToString([]byte("0123456789abcdef")),
		"teaPlain":         hex.EncodeToString(teaSrc),
		"teaCipher":        hex.EncodeToString(teaDst),
		"teaBody":          hex.EncodeToString([]byte("hello-qmc")),
		"teaBodyCipher":    "",
		"mapKey":           hex.EncodeToString(mapKey),
		"mapEkey":          mapEkey,
		"mapAudio":         hex.EncodeToString(audio),
		"mapEncrypted":     hex.EncodeToString(mapWin),
		"mapEncrypted9000": hex.EncodeToString(mapOff),
		"embeddedFile":     hex.EncodeToString(file),
		"qtagFile":         hex.EncodeToString(qFile),
		"stagFile":         hex.EncodeToString(sFile),
		"stagPayload":      stagPayload,
		"rc4Key":           hex.EncodeToString(rc4Key),
		"rc4Ekey":          rc4Ekey,
		"rc4PlainSha":      hex.EncodeToString(plainSum[:]),
		"rc4CipherSha":     hex.EncodeToString(sum[:]),
		"rc4CipherHead":    hex.EncodeToString(rc4Out[:64]),
		"rc4Window":        hex.EncodeToString(rc4Win[:32]),
		"rc4PlainHead":     hex.EncodeToString(plainRC4[:32]),
		"staticPlain":      hex.EncodeToString(staticPlain),
		"staticCipher":     hex.EncodeToString(staticCipher),
		"v2Ekey":           v2ekey,
	}
	bodyCipher, err := encryptTencentTea([]byte("hello-qmc"), []byte("0123456789abcdef"))
	must(err)
	back, err := decryptTencentTea(bodyCipher, []byte("0123456789abcdef"))
	must(err)
	if string(back) != "hello-qmc" {
		panic(string(back))
	}
	vec["teaBodyCipher"] = hex.EncodeToString(bodyCipher)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(vec))
	fmt.Fprintf(os.Stderr, "ok simple=%x tea=%x mapEkeyLen=%d rc4EkeyLen=%d file=%d\n",
		simple, teaDst, len(mapEkey), len(rc4Ekey), len(file))
}
