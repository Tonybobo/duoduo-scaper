package hlsdl

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const (
	syncByte = uint8(71)
)

func (hls *HlsDL) decrypt(segment *Segment) ([]byte, error) {
	file, err := os.Open(segment.Path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	data, err := io.ReadAll(file)

	if err != nil {
		return nil, err
	}

	if segment.Key != nil {
		key, iv, err := hls.getKey(segment)
		if err != nil {
			return nil, err
		}
		data, err = decryptAES128(data, key, iv)

		if err != nil {
			return nil, err
		}
	}
	// every ts files start with 0x47 bits. Need to remove these bits
	for j := 0; j < len(data); j++ {
		if data[j] == syncByte {
			data = data[j:]
			break
		}
	}
	return data, nil
}

func decryptAES128(crypted, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, iv[:blockSize])
	originalData := make([]byte, len(crypted))
	blockMode.CryptBlocks(originalData, crypted)
	originalData = pkcs5Unpadding(originalData)
	return originalData, nil

}

func pkcs5Unpadding(data []byte) []byte {
	length := len(data)
	unPadding := int(data[length-1])
	return data[:(length - unPadding)]
}

func (hls *HlsDL) getKey(segment *Segment) (key []byte, iv []byte, err error) {
	res, err := hls.client.Get(segment.Key.URI)
	if err != nil {
		return nil, nil, err
	}

	if res.StatusCode != 200 {
		return nil, nil, errors.New("fail to get decryption key")
	}

	key, err = io.ReadAll(res.Body)

	if err != nil {
		return nil, nil, err
	}

	iv = []byte(segment.Key.IV)
	if len(iv) == 0 {
		iv = defaultIV(segment.SeqId)
	}

	return
}

func defaultIV(seqID uint64) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[8:], seqID)
	return buf
}
