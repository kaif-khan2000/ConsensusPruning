package main

import (
	ckm "IOT_App/Crypt_key_Manager"
	dh "IOT_App/Data_Handler"
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

var (
	CSS_Address string
	SN_Address  string
)

type SessionInfo struct {
	// session Id
	SessionId [32]byte
	// Total No.of chunks in the session
	TotalChunks int
	// No.of chunks requested
	ChunksRequested int
}

type CssInfo struct {
	// session Ids
	Sessions []SessionInfo
}

type CssData struct {
	SessionID     string
	IhSequence    int
	ChunkPosition int
	RecordArray   []dh.Record
	Ihtx_id       string
	IhStart       int
	IhEnd         int
	Mhtx_id       string
}

func getInfoFromCSS(gw_id string, startTime int64, endTime int64) (CssInfo, error) {
	var cssInfo CssInfo
	// make http get request to CSS
	resp, err := http.Get("http://" + CSS_Address + "/getInfo/" + gw_id + "/" + fmt.Sprint(startTime) + "/" + fmt.Sprint(endTime))
	if err != nil {
		log.Fatalln(err)
		return cssInfo, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
		return cssInfo, err
	}

	dh.Deserialize(string(body), &cssInfo)
	return cssInfo, nil
}

func getDataFromCSS(sessionId [32]byte, startTime int64, endTime int64) ([]CssData, error) {
	var cssData []CssData
	session_id := hex.EncodeToString(sessionId[:])

	// make http get request to CSS
	resp, err := http.Get("http://" + CSS_Address + "/getChunks/" + string(session_id) + "/" + fmt.Sprint(startTime) + "/" + fmt.Sprint(endTime))
	if err != nil {
		log.Fatalln(err)
		return cssData, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
		return cssData, err
	}

	dh.Deserialize(string(body), &cssData)
	return cssData, nil
}

func getAllFromCss(sessionId [32]byte) ([]CssData, error) {
	var cssData []CssData
	session_id := hex.EncodeToString(sessionId[:])

	// make http get request to CSS
	resp, err := http.Get("http://" + CSS_Address + "/getAllChunks/" + string(session_id))
	if err != nil {
		log.Fatalln(err)
		return cssData, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
		return cssData, err
	}

	dh.Deserialize(string(body), &cssData)
	fmt.Println("cssData size: ", len(cssData))
	return cssData, nil
}

func getIhFromSN(ihtx_id_array []string) (map[int]string, error) {
	ih_array := make(map[int]string, 0)
	// HTTP endpoint
	posturl := "http://" + SN_Address + "/getIH"

	jsonData, err := dh.Serialize(ihtx_id_array)
	if err != nil {
		fmt.Println("Error in serializing the data: ", err)
		return ih_array, err
	}
	// send the data to the css
	resp, err := http.Post(posturl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error in sending the data to the SN: ", err)
		return ih_array, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error in reading the response from the SN: ", err)
		return ih_array, err
	}
	_ = dh.Deserialize(string(body), &ih_array)
	return ih_array, nil
}

func getMhFromSN(mhtx_id string) (string, error) {
	var mh string
	// make http get request to CSS
	resp, err := http.Get("http://" + SN_Address + "/getMH/" + string(mhtx_id))
	fmt.Println("http://" + SN_Address + "/getMH/" + string(mhtx_id))
	if err != nil {
		log.Fatalln(err)
		return mh, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
		return mh, err
	}

	_ = dh.Deserialize(string(body), &mh)
	return mh, nil

}

func verifyIH(data []CssData) (bool, []int) {

	// for debug purpose
	// create a file
	f, err := os.Create("error.txt")
	if err != nil {
		fmt.Println(err)
		return false, nil
	}
	defer f.Close()

	err_ih_sequence := make([]int, 0)
	// sort the chunks based on the chunk sequence and chunk position
	sort.Slice(data, func(i, j int) bool {
		if data[i].IhSequence == data[j].IhSequence {
			return data[i].ChunkPosition < data[j].ChunkPosition
		} else {
			return data[i].IhSequence < data[j].IhSequence
		}
	})
	// remove extra chunks
	if data[0].ChunkPosition != 0 {
		data = data[1:]
	}
	if data[len(data)-1].ChunkPosition != 1 {
		data = data[:len(data)-1]
	}
	fmt.Println("ihtx_id:", data[0].Ihtx_id)
	for i := 0; i < len(data); i += 1 {
		fmt.Print(data[i].IhSequence, data[i].ChunkPosition, ", ")
	}
	fmt.Println()

	// get IH from SN
	var ihtx_array []string
	for i := 0; i < len(data); i++ {
		ihtx_array = append(ihtx_array, data[i].Ihtx_id)
	}
	ihFromSN, err := getIhFromSN(ihtx_array)
	if err != nil {
		fmt.Println("Error in getting the IH from SN: ", err)
		return false, err_ih_sequence
	}

	// verify IHs
	for i := 0; i < len(data); i += 2 {
		// sort the records based on the timestamp
		// sort.Slice(data[i].RecordArray, func(k, l int) bool {
		// 	return data[i].RecordArray[k].Timestamp < data[i].RecordArray[l].Timestamp
		// })

		// // sort the records based on the timestamp
		// sort.Slice(data[i+1].RecordArray, func(k, l int) bool {
		// 	return data[i+1].RecordArray[k].Timestamp < data[i+1].RecordArray[l].Timestamp
		// })

		if data[i].IhSequence != data[i+1].IhSequence {
			fmt.Println("Error: IH sequence mismatch")
			// write the error to the file
			_, err = f.WriteString("Error: IH sequence mismatch")
			if err != nil {
				fmt.Println(err)
				f.Close()
				return false, err_ih_sequence
			}
			i--
			continue
		}
		//create a chunk
		chunk1 := dh.Chunk{
			ChunkData: data[i].RecordArray,
			ChunkSize: len(data[i].RecordArray),
		}
		chunk1.ChunkID = ckm.GenerateHashOfObject(chunk1)
		chunk1Hash := hex.EncodeToString(chunk1.ChunkID[:])
		chunk2 := dh.Chunk{
			ChunkData: data[i+1].RecordArray,
			ChunkSize: len(data[i+1].RecordArray),
		}
		chunk2.ChunkID = ckm.GenerateHashOfObject(chunk2)
		chunk2Hash := hex.EncodeToString(chunk2.ChunkID[:])

		ihSequence := data[i].IhSequence
		// now concatenate the two hashes
		concatenatedHash := chunk1Hash + chunk2Hash

		// now generate the hash of the concatenated hash
		ihHash := ckm.GenerateHashOfString(concatenatedHash)
		hash_str := hex.EncodeToString(ihHash[:])
		if err != nil {
			fmt.Println("Error in encoding the hash: ", err)
			return false, err_ih_sequence
		}
		if ihFromSN[ihSequence] == "" {
			fmt.Println("ihSequenceFail not present: ", ihSequence)
			continue
		}
		if hash_str != ihFromSN[ihSequence] {
			err_ih_sequence = append(err_ih_sequence, data[i].IhSequence)
			fmt.Println("ihSequenceFail: ", ihSequence)
			fmt.Println("hash_str: ", hash_str)
			fmt.Println("ihFromSN[ihSequence]: ", ihFromSN[ihSequence])
			// write to the file
			_, err = f.WriteString("ihSequenceFail: " + strconv.Itoa(ihSequence) + "\n")
			c1, err := dh.Serialize(chunk1)
			if err != nil {
				fmt.Println("Error in serializing the chunk1: ", err)
				return false, err_ih_sequence
			}

			ihtx_array_str, _ := dh.Serialize(ihtx_array)
			_, _ = f.WriteString("ihtx_array: " + string(ihtx_array_str) + "\n")
			_, _ = f.WriteString("chunk1:" + string(c1) + "\n")
			c2, err := dh.Serialize(chunk2)
			if err != nil {
				fmt.Println("Error in serializing the chunk2: ", err)
				return false, err_ih_sequence
			}
			_, _ = f.WriteString("chunk2:" + string(c2) + "\n")
			_, _ = f.WriteString("ihHash: " + hex.EncodeToString(ihHash[:]) + "\n")
			_, _ = f.WriteString("----------------------------------------------------------------\n\n")
		}
	}

	fmt.Println()

	if len(err_ih_sequence) == 0 {
		return true, nil
	}
	return false, err_ih_sequence
}

func generateMerkleRoot(ihHash [][32]byte) [32]byte {
	if len(ihHash) < 1 {
		return [32]byte{0}
	}
	if len(ihHash) == 1 {
		return ihHash[0]
	} else {
		var tempHashArray [][32]byte
		for i := 0; i < len(ihHash); i = i + 2 {
			// get the first two hashes from the intermediate hash array
			ih1 := ihHash[i]
			ih2 := [32]byte{0}
			if (i + 1) < len(ihHash) {
				ih2 = ihHash[i+1]
			}

			// now convert the intermediate hash to string
			ih1Hash := hex.EncodeToString(ih1[:])
			ih2Hash := hex.EncodeToString(ih2[:])

			// now concatenate the two hashes
			concatenatedHash := ih1Hash + ih2Hash

			// now generate the hash of the concatenated hash
			ihHash := ckm.GenerateHashOfString(concatenatedHash)

			// now add the hash to the intermediate hash array
			tempHashArray = append(tempHashArray, ihHash)
		}
		return generateMerkleRoot(tempHashArray)
	}
}

func verifyMH(data []CssData) bool {
	// for Ih array using the data
	var chunkArray []dh.Chunk
	var ihHashes [][32]byte
	for i := 0; i < len(data); i++ {
		chunk := dh.Chunk{
			ChunkData: data[i].RecordArray,
			ChunkSize: len(data[i].RecordArray),
		}
		chunk.ChunkID = ckm.GenerateHashOfObject(chunk)
		chunkArray = append(chunkArray, chunk)

		if len(chunkArray) < 2 {
			continue
		}
		// get the first two chunks from the chunk array
		chunk1 := chunkArray[0]
		chunk2 := chunkArray[1]
		// currentSession := session.Id
		//delete the first two chunks from the chunk array
		chunkArray = chunkArray[2:]

		// now convert the chunk hash to string
		chunk1Hash := hex.EncodeToString(chunk1.ChunkID[:])
		chunk2Hash := hex.EncodeToString(chunk2.ChunkID[:])

		// now concatenate the two hashes
		concatenatedHash := chunk1Hash + chunk2Hash

		// now generate the hash of the concatenated hash
		ihHash := ckm.GenerateHashOfString(concatenatedHash)

		// now add the hash to the intermediate hash array
		ihHashes = append(ihHashes, ihHash)
	}

	mh := generateMerkleRoot(ihHashes)
	fmt.Println("mh:", hex.EncodeToString(mh[:]))
	fmt.Println("mhtx_id:", data[0].Mhtx_id)
	// _ = getMhFromSN(data[0].SessionID)
	mhFromSN, err := getMhFromSN(data[0].Mhtx_id)
	if err != nil {
		fmt.Println("Error getting MH from SN")
		return false
	}

	fmt.Println("mhFromSN:", mhFromSN)

	return (hex.EncodeToString(mh[:]) == mhFromSN)
	// return true
}

func verifySession(sessionInfo SessionInfo, threshold float64, startTime int64, endTime int64) bool {
	var data []CssData
	var err error
	fmt.Println("TotalChunks:", sessionInfo.TotalChunks, "ChunksRequested:", sessionInfo.ChunksRequested)
	fmt.Println("Threshold:", float64(sessionInfo.ChunksRequested)/float64(sessionInfo.TotalChunks))
	if sessionInfo.TotalChunks == 0 {
		fmt.Println("No data to verify")
		return false
	}
	if float64(sessionInfo.ChunksRequested)/float64(sessionInfo.TotalChunks) <= threshold {
		time1 := time.Now()
		// get data from CSS
		data, err = getDataFromCSS(sessionInfo.SessionId, startTime, endTime)
		if err != nil {
			fmt.Println("Error getting data from CSS")
		}
		ok, ihseq := verifyIH(data)
		time2 := time.Now()
		fmt.Println("app: Time taken for IH verification: ", threshold, sessionInfo.ChunksRequested, time2.Sub(time1))
		if ok {
			fmt.Println("data is not tampered")
			return true
		} else {
			fmt.Println("data is tampered", ihseq)
			return false
			// also mark the data as tampered in CSS or SN
		}

	} else {
		// get all data from CSS
		time1 := time.Now()
		data, err = getAllFromCss(sessionInfo.SessionId)
		if err != nil {
			fmt.Println("Error getting data from CSS")
		}

		if verifyMH(data) {
			fmt.Println("data is not tampered")
			time2 := time.Now()
			fmt.Println("app: Time taken for MH verification: ", threshold, sessionInfo.ChunksRequested, time2.Sub(time1))
			return true
		} else {
			fmt.Println("data is tampered, checking with IH")
			// get data from CSS
			data, err = getDataFromCSS(sessionInfo.SessionId, startTime, endTime)
			if err != nil {
				fmt.Println("Error getting data from CSS")
			}
			ok, ihseq := verifyIH(data)
			time2 := time.Now()
			fmt.Println("app: Time taken for MH+IH verification: ", threshold, sessionInfo.ChunksRequested, time2.Sub(time1))
			if ok {
				fmt.Println("data is not tampered with IH")
				return true
			} else {
				fmt.Println("data is tampered with IH", ihseq)
				return false
				// also mark the data as tampered in CSS or SN
			}
			// also mark the data as tampered in CSS or SN
		}
	}
}

func verify(startTime int64, endTime int64, threshold float64) {
	var gw_id string

	CSS_Address = "localhost:9090"
	SN_Address = "150.0.0.2:4000"

	// // read from user
	// fmt.Print("Enter the gateway ID: ")
	// fmt.Scanln(&gw_id)
	// fmt.Print("Enter the start time: ")
	// fmt.Scanln(&startTime)
	// fmt.Print("Enter the end time: ")
	// fmt.Scanln(&endTime)
	gw_id = "tbx-fx-A08190075"

	// endTime = 1675924943849277
	// endTime = 1679628133968422
	// threshold := 0.5

	// get info from CSS
	cssInfo, err := getInfoFromCSS(gw_id, startTime, endTime)
	fmt.Println("css sessions:", len(cssInfo.Sessions))
	if err != nil {
		fmt.Println("Error getting info from CSS")
	}

	fmt.Println(cssInfo)

	for _, sessionInfo := range cssInfo.Sessions {
		fmt.Println()
		verifySession(sessionInfo, threshold, startTime, endTime)
		fmt.Println()
	}
	// process data
}

func main() {
	startTime := int64(1684309594941855)
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Minute)
		endTime := time.Now().UnixMicro()
		time.Sleep(30 * time.Second)
		verify(startTime, endTime, 0.5)
	}

	// startTime := int64(1684309594941855)
	// endTime := int64(1684310034293948)

	// verify(startTime, endTime, 0.5)

}
