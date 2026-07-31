package simpleproxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	cryptoKey  []byte
	cryptoKeyMu sync.RWMutex
)

// SetCryptoKey sets the AES-128-CBC key from a hex-encoded string (32 hex chars = 16 bytes).
func SetCryptoKey(hexKey string) error {
	if hexKey == "" {
		return nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return fmt.Errorf("invalid crypto_key hex: %w", err)
	}
	if len(key) != 16 {
		return errors.New("crypto_key must be exactly 16 bytes (32 hex characters)")
	}
	cryptoKeyMu.Lock()
	cryptoKey = key
	cryptoKeyMu.Unlock()
	return nil
}

// InitCrypto initializes the crypto key. If no key is configured, generates a random one.
func InitCrypto(configuredKey string) error {
	if configuredKey != "" {
		return SetCryptoKey(configuredKey)
	}
	key := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("failed to generate random crypto key: %w", err)
	}
	cryptoKeyMu.Lock()
	cryptoKey = key
	cryptoKeyMu.Unlock()
	return nil
}

func getKey() []byte {
	cryptoKeyMu.RLock()
	defer cryptoKeyMu.RUnlock()
	return cryptoKey
}

// pkcs7Pad pads the data to a multiple of the block size using PKCS7 padding.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := make([]byte, len(data)+padding)
	copy(padText, data)
	for i := len(data); i < len(padText); i++ {
		padText[i] = byte(padding)
	}
	return padText
}

// pkcs7Unpad removes PKCS7 padding from the data.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	if len(data)%blockSize != 0 {
		return nil, errors.New("invalid padding: data length not a multiple of block size")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize {
		return nil, errors.New("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

// Encrypt encrypts plaintext using AES-128-CBC with PKCS7 padding.
// Returns hex-encoded ciphertext.
// The IV is prepended to the ciphertext (first 16 bytes of the hex output).
func Encrypt(plaintext string) (string, error) {
	key := getKey()
	if key == nil {
		return "", errors.New("crypto key not initialized")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// Generate random IV
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	// Prepend IV to ciphertext
	result := make([]byte, len(iv)+len(ciphertext))
	copy(result, iv)
	copy(result[len(iv):], ciphertext)

	return hex.EncodeToString(result), nil
}

// Decrypt decrypts a hex-encoded ciphertext (IV prepended) using AES-128-CBC.
func Decrypt(cipherHex string) (string, error) {
	key := getKey()
	if key == nil {
		return "", errors.New("crypto key not initialized")
	}

	data, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("invalid hex: %w", err)
	}

	if len(data) < aes.BlockSize {
		return "", errors.New("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	iv := data[:aes.BlockSize]
	ciphertext := data[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("ciphertext length not a multiple of block size")
	}

	padded := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(padded, ciphertext)

	plaintext, err := pkcs7Unpad(padded, aes.BlockSize)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
