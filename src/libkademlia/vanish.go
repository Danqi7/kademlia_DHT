package libkademlia

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	mathrand "math/rand"
	"time"
	"sss"
	"log"
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

func CalculateSharedKeyLocations(accessKey int64, count int64, lastTimeOut int64) (ids []ID) {
	r := mathrand.New(mathrand.NewSource(accessKey * lastTimeOut))
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

func (ka *Kademlia) VanishData(vdoID ID, data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	K := GenerateRandomCryptoKey()
	ciphertext := encrypt(K, data)

	sssMap, err := sss.Split(numberKeys, threshold, K)
	if err != nil {
 		log.Printf("Vanish cannot split key: %d/%d\n", threshold, numberKeys)
 		return VanashingDataObject{}
 	}

	L := GenerateRandomAccessKey()
	ka.VanishLastTimeOut = time.Now().UnixNano()
	ids := CalculateSharedKeyLocations(L, int64(numberKeys), ka.VanishLastTimeOut)

	vdo = VanashingDataObject{
		AccessKey: L,
		Ciphertext: ciphertext,
		NumberKeys: numberKeys,
		Threshold: threshold,
	}

	timeoutCh := make(chan bool)
	// refresh key shares and sprinkles after each timeout
	go func() {
		for {
			time.Sleep(3000 * time.Millisecond) // check timeout every 30 seconds
			if (time.Now().UnixNano() - ka.VanishLastTimeOut) / 1000000000 > int64(timeoutSeconds) {
				timeoutCh <- true
			}
		}
	}()

	// listen to timeout and refresh key shares and sprinkles
	go func() {
		for {
			select {
			case <- timeoutCh:
				newK := GenerateRandomCryptoKey()
				newCipher := encrypt(newK, data)
				newSSSMap, err := sss.Split(numberKeys, threshold, newK)
				if err != nil {
					log.Printf("Vanish cannot split key when refreshing: %d/%d\n", threshold, numberKeys)
				}
				// update vanishLastTimeOut
				ka.VanishLastTimeOut = time.Now().UnixNano()
				newL := GenerateRandomAccessKey()
				newIDs := CalculateSharedKeyLocations(newL, int64(numberKeys), ka.VanishLastTimeOut)
				vdo = VanashingDataObject{
					AccessKey: newL,
					Ciphertext: newCipher,
					NumberKeys: numberKeys,
					Threshold: threshold,
				}

				// sprinkle as always
				i := 0
				for key, val := range(newSSSMap) {
					all := append([]byte{key}, val...)
					//just use the contact id as the key for the stored share in table
					storeAddr := CopyID(newIDs[i])
					_, err := ka.DoIterativeStore(storeAddr, all)

					if err != nil {
						log.Printf("Vanish refreshing DoIterativeStore Error: %s\n", err.Error())
					}

					i += 1
				}
				// update vdo
				ka.semVdos <- 1
				ka.Vdos[vdoID] = vdo
				<- ka.semVdos

				log.Println("REFRESHING SUCCEED.")

			}
		}
	}()

	// sprinkle splits to ids, initial case
	i := 0
	for key, val := range(sssMap) {
		all := append([]byte{key}, val...)
		//just use the contact id as the key for the stored share in table
		storeAddr := CopyID(ids[i])
		_, err := ka.DoIterativeStore(storeAddr, all)

		if err != nil {
			log.Printf("Vanish DoIterativeStore Error: %s\n", err.Error())
	 		return VanashingDataObject{}
		}

		i += 1
	}

	return vdo
}

func (ka *Kademlia) UnvanishData(vdo VanashingDataObject) (data []byte) {
	L := vdo.AccessKey
	numberKeys := vdo.NumberKeys
	threshold := vdo.Threshold

	keyShares := make(map[byte][]byte)
	cnt := 0
	ids := CalculateSharedKeyLocations(L, int64(numberKeys), ka.VanishLastTimeOut)
	for _, id := range ids {
		storedKey := CopyID(id)
		value, err := ka.DoIterativeFindValue(storedKey)
		if err != nil {
			log.Printf("Unvanish DoIterativeFindValue no value found for this key: %s\n", storedKey.AsString())
		} else {
			// find a share
			k := value[0]
			v := value[1:]
			keyShares[k] = v

			cnt += 1
		}

		// only need to find threshold number of shares to decrypt
		if cnt >= int(threshold) {
			break
		}
	}

	// not enough shares are found, can't decrypt!
	if cnt < int(threshold) {
		log.Printf("Only %d shares are found, cannot decrypt\n", cnt)
		return nil
	}

	K := sss.Combine(keyShares)
	decryptText := decrypt(K, vdo.Ciphertext)

	return decryptText
}
