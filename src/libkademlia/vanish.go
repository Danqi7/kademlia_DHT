package libkademlia

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	mathrand "math/rand"
	"time"
	//"sss"
)

type VanashingDataObject struct {
	AccessKey  int64
	Ciphertext []byte
	NumberKeys byte
	Threshold  byte
}

func GenerateRandomCryptoKey() (ret []byte) {
	for i := 0; i < 32; i++ {
		ret = append(ret, uint8(mathrand.Intn(256)))
	}
	return
}

func GenerateRandomAccessKey() (accessKey int64) {
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	accessKey = r.Int63()
	return
}

func CalculateSharedKeyLocations(accessKey int64, count int64) (ids []ID) {
	r := mathrand.New(mathrand.NewSource(accessKey))
	ids = make([]ID, count)
	for i := int64(0); i < count; i++ {
		for j := 0; j < IDBytes; j++ {
			ids[i][j] = uint8(r.Intn(256))
		}
	}
	return
}

func encrypt(key []byte, text []byte) (ciphertext []byte) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	ciphertext = make([]byte, aes.BlockSize+len(text))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], text)
	return
}

func decrypt(key []byte, ciphertext []byte) (text []byte) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	if len(ciphertext) < aes.BlockSize {
		panic("ciphertext is not long enough")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	return ciphertext
}

func (ka *Kademlia) VanishData(data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {

	// TIMEOUT is currently ignored as it is relevant only in extra credit

	/*
	 * first start by encrypting the data and creating a ciphertext.
	 * To create a random cryptographic key (K),
	 * you can use the provided "GenerateRandomCryptoKey" function.
	 * To encrypt data, you can use the provided "encrypt" function by
	 * giving it the random key (K) and text that shoudl be encrypted.
	 */

	k := GenerateRandomCryptoKey()
	cypherText := encrypt(k, data)

	/*
	 * The key, K, should then be broken up into multiple
	 * Shamir's Secret Shared keys using sss.Split in sss/sss.go.
	 */
	sssMap, err := Split(numberKeys, threshold, cypherText)
	if err != nil {
		log.Printf("Vanish cannot split key: %d/%d\n", threshold, numberKeys)
		return nil
	}

	/*
	 * Next, these keys need to be stored in the DHT.
	 * For this step, you need to create an access key (L).
	 * For this you can use the provided GenerateRandomAccessKey.
	 * This will be used as the seed to the random psuedo number generator
	 * that we use to calculate where to store the shared keys in the DHT.
	 * This step is done for you in the CalculateSharedKeyLocations function.
	 * Given the same accessKey, this function will return the same sequence
	 * of N random IDs -- locations in the DHT where the keys should be stored.
	 * Using your previously implemented Kademlia functionality,
	 * you should store these shared keys in the network.
	 */

	l := GenerateRandomAccessKey()
	ids := CalculateSharedKeyLocations(l, numberKeys)
	for i := 0; i < numberKeys; i++ {
		ka.DoIterativeStore(ids[i], sssMap[i])
	}

	/*
	 * Finally, after the keys have been stored, you should create and return the VDO.
	 * Later in this project, you will need to store the VDO locally and serve it via an RPC.
	 * For now, however, you can save it however you prefer for testing.
	 */
	return VanashingDataObject(l, cypherText, numberKeys, threshold)
}

func (ka *Kademlia) UnvanishData(vdo VanashingDataObject) (data []byte) {
	return nil
}
