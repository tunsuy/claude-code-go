package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
)

// fileStoreVersion is a 4-byte version prefix for the encrypted file format.
// Allows future algorithm migration without breaking existing files.
const fileStoreVersion = uint32(1)

// fileStoreIterations is the PBKDF2 iteration count for key derivation.
const fileStoreIterations = 100_000

// encryptTokenFile encrypts plaintext with AES-256-GCM.
// Output format: [4-byte version][32-byte salt][12-byte nonce][ciphertext+16-byte GCM tag]
func encryptTokenFile(plaintext []byte) ([]byte, error) {
	key, salt, err := deriveKey(nil)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 4+len(salt)+len(nonce)+len(ciphertext))
	binary.BigEndian.PutUint32(out[:4], fileStoreVersion)
	copy(out[4:], salt)
	copy(out[4+len(salt):], nonce)
	copy(out[4+len(salt)+len(nonce):], ciphertext)
	return out, nil
}

// decryptTokenFile decrypts a file encrypted by encryptTokenFile.
func decryptTokenFile(data []byte) ([]byte, error) {
	if len(data) < 4+32+12+16 {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}

	version := binary.BigEndian.Uint32(data[:4])
	if version != fileStoreVersion {
		return nil, fmt.Errorf("crypto: unsupported file version %d", version)
	}

	salt := data[4 : 4+32]
	nonceStart := 4 + 32
	nonceLen := 12
	nonce := data[nonceStart : nonceStart+nonceLen]
	ciphertext := data[nonceStart+nonceLen:]

	key, _, err := deriveKey(salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// deriveKey derives a 32-byte AES key using PBKDF2-SHA256.
// If salt is nil, a fresh 32-byte random salt is generated.
// The password is derived from a machine-specific identifier (hostname).
func deriveKey(salt []byte) (key, usedSalt []byte, err error) {
	if salt == nil {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, fmt.Errorf("crypto: salt: %w", err)
		}
	}
	password := machinePassword()
	key = pbkdf2Key(password, salt, fileStoreIterations, 32)
	return key, salt, nil
}

// pbkdf2Key is an inline PBKDF2-SHA256 implementation (RFC 2898).
// Avoids external golang.org/x/crypto dependency.
func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	dk := make([]byte, numBlocks*hashLen)
	var buf [4]byte

	for block := 1; block <= numBlocks; block++ {
		// U1 = PRF(password, salt || INT(block))
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])

		U := prf.Sum(nil) // U1
		T := dk[(block-1)*hashLen : block*hashLen]
		copy(T, U)

		// Un = PRF(password, U(n-1)); T ^= Un
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(U)
			U = prf.Sum(U[:0])
			for x := range T {
				T[x] ^= U[x]
			}
		}
	}
	return dk[:keyLen]
}

// machinePassword returns a machine-specific password for key derivation.
// Uses hostname as the primary source; falls back to a static string.
// This ensures tokens.enc is bound to the machine it was created on.
func machinePassword() []byte {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "claude-code-fallback-host"
	}
	return []byte("claude-code-token-key-v1:" + hostname)
}
