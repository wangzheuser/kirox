package crypto

import (
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"sync"
)

const (
	delta       uint32 = 0x9E3779B9
	fallbackVer        = "4.0.0"
	identifier         = "ECdITeCs"
)

var (
	fallbackKey = [4]uint32{1888420705, 2576816180, 2347232058, 874813317}

	cacheMu          sync.Mutex
	cachedKey        *[4]uint32
	cachedVersion    string
	cachedIdentifier string
)

func GetTESVersion() string {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedVersion != "" {
		return cachedVersion
	}
	return fallbackVer
}

func GetIdentifier() string {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedIdentifier != "" {
		return cachedIdentifier
	}
	return identifier
}

func GetActiveKey() [4]uint32 {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedKey != nil {
		return *cachedKey
	}
	return fallbackKey
}

// EncryptFingerprint 加密指纹 JSON 字符串
func EncryptFingerprint(jsonStr string) string {
	crc := crc32.ChecksumIEEE([]byte(jsonStr))
	plaintext := fmt.Sprintf("%08X#%s", crc, jsonStr)
	key := GetActiveKey()
	encrypted := xxteaEncrypt(plaintext, key)
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	return GetIdentifier() + ":" + encoded
}

func xxteaEncrypt(plaintext string, key [4]uint32) []byte {
	if len(plaintext) == 0 {
		return nil
	}
	n := (len(plaintext) + 3) / 4
	v := make([]uint32, n)
	for i := 0; i < n; i++ {
		var b0, b1, b2, b3 byte
		if 4*i < len(plaintext) {
			b0 = plaintext[4*i]
		}
		if 4*i+1 < len(plaintext) {
			b1 = plaintext[4*i+1]
		}
		if 4*i+2 < len(plaintext) {
			b2 = plaintext[4*i+2]
		}
		if 4*i+3 < len(plaintext) {
			b3 = plaintext[4*i+3]
		}
		v[i] = uint32(b0) | uint32(b1)<<8 | uint32(b2)<<16 | uint32(b3)<<24
	}
	rounds := 6 + 52/n
	z := v[n-1]
	var total uint32
	for r := 0; r < rounds; r++ {
		total += delta
		e := (total >> 2) & 3
		for p := 0; p < n; p++ {
			y := v[(p+1)%n]
			mx := ((z>>5 ^ y<<2) + (y>>3 ^ z<<4)) ^ ((total ^ y) + (key[(uint32(p)&3)^e] ^ z))
			v[p] += mx
			z = v[p]
		}
	}
	result := make([]byte, n*4)
	for i, val := range v {
		result[4*i] = byte(val)
		result[4*i+1] = byte(val >> 8)
		result[4*i+2] = byte(val >> 16)
		result[4*i+3] = byte(val >> 24)
	}
	return result
}
