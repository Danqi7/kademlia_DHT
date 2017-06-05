package libkademlia

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"log"
	mathrand "math/rand"
	"sss"
	"time"
)

/*
 * Extra Credit Implementation Notes:
 *
 * Concept
 * - epoch: a period of time of prespecified length.
 *          The first epoch starts at Jan. 1 1970 00:00:00
 *
 * Constants
 * - epoch_in_seconds: 10 second per epoch, or 8,640 epoches a day
 * - time_beore_resplit: at the 8th second, we call for a resplit, if needed
 *
 * We want to stick to the "do-nothing" expiration policy, so
 * 1. We included the epoch number in the computation of split key locations,
 *    through setting the seed of the pseudo-random number generator to be
 *    the product of the accessKey and the epoch number of current time
 * 2. We adopted a very weak notion of expiration in favor of retrieval:
 *    if the timeout ends at the beginning of an epoch, users would still
 *    be able to retrieve the data until the end of the epoch
 * 3. At vanish time, two go-routines are kicked off:
 *    One to check periodically whether the end of an epoch will come before timeout;
 *    if so it will signal the other go-routine to apply a resplit
 * 4. We modified the key recovery code to use the current and, if doing so fails,
 *    the immediate next epoch to compute the split keys. We do so because
 *    when nodes are few and we resplit the encryption key, the split key
 *    stored remotely under the same vdoID gets overwritten -- if an unvanish
 *    is issued between the time of our resplit and the start of the next new epoch,
 *    the user would be unable to retrieve the data.
 */
const (
	epoch_in_seconds    = 10 // number of seconds in an epoch. For example, 8-hour epoch = 8 * 60 * 60 = 28,800 seconds
	time_before_resplit = 8  // after this many seconds after the start of an epoch, a resplit happens
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
	r := mathrand.New(mathrand.NewSource(accessKey * calculateCurrentEpoch()))
	ids = make([]ID, count)
	for i := int64(0); i < count; i++ {
		for j := 0; j < IDBytes; j++ {
			ids[i][j] = uint8(r.Intn(256))
		}
	}
	return
}

func CalculateSharedKeyLocationsNextEpoch(accessKey int64, count int64) (ids []ID) {
	r := mathrand.New(mathrand.NewSource(accessKey * calculateNextEpoch()))
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

func (ka *Kademlia) acquireKeyShares(isCurrentEpoch bool, vdo VanashingDataObject) (map[byte][]byte, int) {

	L := vdo.AccessKey
	numberKeys := int64(vdo.NumberKeys)
	threshold := vdo.Threshold

	keyShares := make(map[byte][]byte)
	cnt := 0

	var ids []ID
	if isCurrentEpoch {
		ids = CalculateSharedKeyLocations(L, numberKeys)
	} else {
		ids = CalculateSharedKeyLocationsNextEpoch(L, numberKeys)
	}
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
		if cnt > int(threshold) {
			break
		}
	}
	return keyShares, cnt
}

func (ka *Kademlia) recoverOriginalKey(vdo VanashingDataObject) []byte {

	keyShares, cnt := ka.acquireKeyShares(true, vdo)
	if cnt <= int(vdo.Threshold) {
		keyShares, cnt = ka.acquireKeyShares(false, vdo)
		if cnt <= int(vdo.Threshold) {
			// not enough shares are found, can't decrypt!
			log.Printf("Only %d shares are found, cannot decrypt\n", cnt)
			return nil
		}
	}

	K := sss.Combine(keyShares)
	return K
}

func calculateCurrentEpoch() int64 {
	current_time := int64(time.Now().UnixNano())
	current_epoch := current_time / (int64(time.Second) * epoch_in_seconds)
	// log.Printf("calculateCurrentEpoch - current time: %d; current epoch: %d\n", current_time, current_epoch)
	return current_epoch
}

func calculateNextEpoch() int64 {
	return calculateCurrentEpoch() + 1
}

func secondsPassedInEpoch() int64 {
	current_second := time.Now().UnixNano() / int64(time.Second)
	return current_second % epoch_in_seconds
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
	ids := CalculateSharedKeyLocations(L, int64(numberKeys))

	vdo = VanashingDataObject{
		AccessKey:  L,
		Ciphertext: ciphertext,
		NumberKeys: numberKeys,
		Threshold:  threshold,
	}

	// sprinkle splits to ids, initial case
	i := 0
	for key, val := range sssMap {
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

	// EC: Timeouts
	timeoutCh := make(chan bool)
	stopCh := make(chan bool)

	go func(timeOutLength int) {
		accrued_time := 0
		for {
			secondsPassedInThisEpoch := secondsPassedInEpoch()
			if secondsPassedInThisEpoch >= time_before_resplit && accrued_time == 0 {
				timeoutCh <- true
				time.Sleep(time.Duration((epoch_in_seconds - secondsPassedInThisEpoch)) * time.Second)
				accrued_time += int(epoch_in_seconds - secondsPassedInThisEpoch)
				log.Printf("Refresh rest time accrued, short notice: %d/%d\n", accrued_time, timeOutLength)
				continue
			}

			time.Sleep(time.Duration((time_before_resplit - secondsPassedInThisEpoch)) * time.Second)
			timeoutCh <- true
			accrued_time += int(time_before_resplit)
			log.Printf("Refresh accrued time: %d/%d\n", accrued_time, timeOutLength)
			if accrued_time+int(epoch_in_seconds) >= timeOutLength {
				stopCh <- true
				log.Printf("Refresh terminated: %d/%d\n", accrued_time, timeOutLength)
				return
			}
			time.Sleep(time.Duration(epoch_in_seconds-time_before_resplit) * time.Second)
			accrued_time += int(epoch_in_seconds - time_before_resplit)
			log.Printf("Refresh rest time accrued: %d/%d\n", accrued_time, timeOutLength)
		}
	}(timeoutSeconds)

	// listen to timeout and refresh key shares and sprinkles
	go func() {
		for {
			select {
			case <-timeoutCh:
				newK := ka.recoverOriginalKey(vdo)
				newSSSMap, err := sss.Split(vdo.NumberKeys, vdo.Threshold, newK)
				if err != nil {
					log.Printf("Vanish cannot split key when refreshing: %d/%d\n", threshold, numberKeys)
					return
				}
				// update vanishLastTimeOut
				newIDs := CalculateSharedKeyLocationsNextEpoch(vdo.AccessKey, int64(vdo.NumberKeys))

				// sprinkle as always
				i := 0
				for key, val := range newSSSMap {
					all := append([]byte{key}, val...)
					//just use the contact id as the key for the stored share in table
					storeAddr := CopyID(newIDs[i])
					_, err := ka.DoIterativeStore(storeAddr, all)

					if err != nil {
						log.Printf("Vanish refreshing DoIterativeStore Error: %s\n", err.Error())
						return
					}

					i += 1
				}
				log.Println("REFRESHING SUCCEED.")
			case <-stopCh:
				return
			}
		}
	}()

	return vdo
}

func (ka *Kademlia) UnvanishData(vdo VanashingDataObject) (data []byte) {
	K := ka.recoverOriginalKey(vdo)
	if K == nil {
		log.Printf("UnvanishData cannot recover the original key\n")
		return nil
	}
	decryptText := decrypt(K, vdo.Ciphertext)
	return decryptText
}
