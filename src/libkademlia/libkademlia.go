package libkademlia

// Contains the core kademlia type. In addition to core state, this type serves
// as a receiver for the RPC methods, which is required by that package.

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
	"time"
	//"sort"
)

const (
	alpha        = 3
	b            = 8 * IDBytes
	k            = 20
	bucketsCount = 160
)

// assume only one goroutine can access whole bucketlist and table
// of a node at one time
// var ka.sem = make(chan int, 1)
// var ka.semTable = make(chan int, 1)

// Kademlia type. You can put whatever state you need in this.
type Kademlia struct {
	NodeID      		ID
	SelfContact 		Contact
	KbucketList 		[]Kbucket
	Table       		map[ID][]byte
	Vdos				map[ID]VanashingDataObject
	sem         		chan int
	semTable    		chan int
	semVdos				chan int //semaphore for Vdos
}

func NewKademliaWithId(laddr string, nodeID ID) *Kademlia {
	ka := new(Kademlia)
	ka.NodeID = nodeID

	// TODO: Initialize other state here as you add functionality.

	// init 160 kbuckets
	ka.KbucketList = make([]Kbucket, bucketsCount)
	for i := 0; i < bucketsCount; i++ {
		ka.KbucketList[i].Init(ka.NodeID)
	}
	// init Table
	ka.Table = make(map[ID][]byte)
	// init Vdos
	ka.Vdos = make (map[ID]VanashingDataObject)

	// init ka.semaphores
	ka.sem = make(chan int, 1)
	ka.semTable = make(chan int, 1)
	ka.semVdos = make(chan int, 1)

	// Set up RPC server
	// NOTE: KademliaRPC is just a wrapper around Kademlia. This type includes
	// the RPC functions.

	s := rpc.NewServer()
	s.Register(&KademliaRPC{ka})
	hostname, port, err := net.SplitHostPort(laddr)
	if err != nil {
		return nil
	}
	s.HandleHTTP(rpc.DefaultRPCPath+port,
		rpc.DefaultDebugPath+port)

	l, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatal("Listen: ", err)
	}

	// Run RPC server forever.
	go http.Serve(l, nil)
	// Add self contact
	hostname, port, _ = net.SplitHostPort(l.Addr().String())
	port_int, _ := strconv.Atoi(port)
	ipAddrStrings, err := net.LookupHost(hostname)
	var host net.IP
	for i := 0; i < len(ipAddrStrings); i++ {
		host = net.ParseIP(ipAddrStrings[i])
		if host.To4() != nil {
			break
		}
	}
	ka.SelfContact = Contact{ka.NodeID, host, uint16(port_int)}
	return ka
}

func NewKademlia(laddr string) *Kademlia {
	return NewKademliaWithId(laddr, NewRandomID())
}

type ContactNotFoundError struct {
	id  ID
	msg string
}

func (e *ContactNotFoundError) Error() string {
	return fmt.Sprintf("%x %s", e.id, e.msg)
}

func (ka *Kademlia) PrintKbucketList() {
	for _, kbucket := range ka.KbucketList {
		kbucket.PrintBucket()
	}
}

func (ka *Kademlia) FindContact(nodeId ID) (*Contact, error) {
	ka.sem <- 1
	if nodeId == ka.SelfContact.NodeID {
		<-ka.sem
		return &ka.SelfContact, nil
	}
	// a comprehensive search for all kbuckets
	// maybe a more efficient way to find contact is to search in the closest kbucket
	for i := 0; i < len(ka.KbucketList); i++ {
		kb := ka.KbucketList[i]
		for j := 0; j < len(kb.ContactList); j++ {
			current := kb.ContactList[j]
			if current.NodeID.Equals(nodeId) {
				<-ka.sem
				return &current, nil
			}
		}
	}
	<-ka.sem
	return nil, &ContactNotFoundError{nodeId, "Not found"}
}

type CommandFailed struct {
	msg string
}

func (e *CommandFailed) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

type NoResponse struct {
	msg string
}

func (e *NoResponse) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (ka *Kademlia) DoPing(host net.IP, port uint16) (*Contact, error) {
	address := host.String() + ":" + strconv.Itoa(int(port))
	path := rpc.DefaultRPCPath + strconv.Itoa(int(port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		//log.Fatal("Dialing: ", err, address)
		return nil, errors.New("Failed to dial address: " + address)
	}

	PingMsg := new(PingMessage)
	PingMsg.Sender = ka.SelfContact
	PingMsg.MsgID = NewRandomID()

	//Dial RPC
	var PongMsg PongMessage
	err = client.Call("KademliaRPC.Ping", PingMsg, &PongMsg)
	if err != nil {
		//log.Fatal("RPC Ping: ", err)
		return nil, errors.New("DoPing Error: " + err.Error())
	}

	if PongMsg.MsgID.Equals(PingMsg.MsgID) {
		//update the responded contact
		update := PongMsg.Sender
		ka.UpdateContact(&update)
		return &update, nil
	}

	//otherwise, no response
	return nil, &NoResponse{"No response from pinged contact"}

}

type StoreError struct {
	msg string
}

func (e *StoreError) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (ka *Kademlia) DoStore(contact *Contact, key ID, value []byte) error {
	// TODO: Implement
	address := contact.Host.String() + ":" + strconv.Itoa(int(contact.Port))
	path := rpc.DefaultRPCPath + strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		//log.Fatal("Dialing: ", err, address)
		return errors.New("Failed to dial address: " + address)
	}

	request := new(StoreRequest)
	request.MsgID = NewRandomID()
	request.Sender = ka.SelfContact
	request.Key = CopyID(key)
	request.Value = value

	var result StoreResult
	err = client.Call("KademliaRPC.Store", request, &result)
	if err != nil {
		//log.Fatal("DoStore: ", err)
		return errors.New("DoStore Errror: " + err.Error())
	}

	if result.MsgID.Equals(request.MsgID) {
		// update contact in kbucket
		ka.UpdateContact(contact)

		if result.Err != nil {
			return &StoreError{"Unsuccessful Store"}
		}

		return nil
	}

	//MsgID mismatch
	return &MsgIDMismatchError{"MsgID mismatch between request and result for DoStore"}

}

type MsgIDMismatchError struct {
	msg string
}

func (e *MsgIDMismatchError) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

type FindNodeError struct {
	msg string
}

func (e *FindNodeError) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (ka *Kademlia) DoFindNode(contact *Contact, searchKey ID) ([]Contact, error) {
	// TODO: Implement
	address := contact.Host.String() + ":" + strconv.Itoa(int(contact.Port))
	path := rpc.DefaultRPCPath + strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		// log.Fatal("Dialing: ", err, address)
		return nil, errors.New("Failed to dial address: " + address)
	}

	request := new(FindNodeRequest)
	request.Sender = ka.SelfContact
	request.NodeID = searchKey
	request.MsgID = NewRandomID()

	var result FindNodeResult
	err = client.Call("KademliaRPC.FindNode", request, &result)
	if err != nil {
		//log.Fatal("FindNode: ", err)
		return nil, errors.New("FindNode Error: " + err.Error())
	}

	if result.Err != nil {
		return nil, &FindNodeError{"result Err from FindNode"}
	}

	// update contact in kbucket
	// add returned contacts
	if result.MsgID.Equals(request.MsgID) {
		//update the responded contact
		ka.UpdateContact(contact)

		// add returned contacts to its kbuckets, don't add the requestor itself
		// might need this, need to double check with fabian
		for _, cont := range result.Nodes {
			if cont.NodeID.Equals(ka.NodeID) {
				continue
			}
			ka.UpdateContact(&cont)
		}

		return result.Nodes, nil
	}

	//MsgID mismatch
	return nil, &MsgIDMismatchError{"MsgID mismatch between request and result for FindNode"}
}

type FindLocalValError struct {
	msg string
}

func (e *FindLocalValError) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

type NoContactsAndValue struct {
	msg string
}

func (e *NoContactsAndValue) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (ka *Kademlia) DoFindValue(contact *Contact,
	searchKey ID) (value []byte, contacts []Contact, err error) {
	address := contact.Host.String() + ":" + strconv.Itoa(int(contact.Port))
	path := rpc.DefaultRPCPath + strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		//log.Fatal("Dialing: ", err, address)
		return nil, nil, errors.New("Failed to dial address: " + address)
	}

	request := new(FindValueRequest)
	request.MsgID = NewRandomID()
	request.Sender = ka.SelfContact
	request.Key = CopyID(searchKey)

	var result FindValueResult
	err = client.Call("KademliaRPC.FindValue", request, &result)
	if err != nil {
		return nil, nil, errors.New("FindValue Error : " + err.Error())
	}

	if result.MsgID.Equals(request.MsgID) {
		//update contact in bucket
		ka.UpdateContact(contact)

		if result.Value != nil {
			return result.Value, nil, nil
		}

		if result.Nodes != nil && len(result.Nodes) != 0 {
			// add returned contacts to its kbuckets, don't add the requestor itself
			// might need this, need to double check with fabian
			for _, cont := range result.Nodes {
				if cont.NodeID.Equals(ka.NodeID) {
					continue
				}
				ka.UpdateContact(&cont)
			}

			return nil, result.Nodes, nil
		}

		log.Println("No contacts and value stored in this node: ", *contact)
		return nil, nil, &NoContactsAndValue{"No contacts and value stored"}
	}

	//MsgID mismatch
	return nil, nil, &MsgIDMismatchError{"MsgID mismatch between request and result for FindNode"}
}

func (ka *Kademlia) LocalFindValue(searchKey ID) ([]byte, error) {
	ka.semTable <- 1
	val, ok := ka.Table[searchKey]
	<-ka.semTable
	if ok == true && val != nil {
		return val, nil
	}

	return []byte(""), &FindLocalValError{"Can't find the value in the local"}
}

type ReturnedContacts struct {
	Contacts []Contact
	Source   Contact
}

/**
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 */

// For project 2!
func (ka *Kademlia) ReachOutForContacts(returnedContactsCh chan ReturnedContacts, c Contact, id ID) {
	// acquire local closest contacts
	log.Println("ReachOutForContacts...")
	contacts, err := ka.DoFindNode(&c, id)
	var res ReturnedContacts
	if err != nil {
		res.Contacts = nil
	} else {
		// remove itself from ReturnedContacts
		for i, con := range contacts {
			if con.NodeID.Equals(ka.NodeID) {
				contacts = append(contacts[:i], contacts[i+1:]...)
				break
			}
		}

		res.Contacts = contacts
	}
	res.Source = c

	returnedContactsCh <- res
}

// Assume DoFindNode always returnss
func (ka *Kademlia) FindNodeCycle(sl *ShortList, id ID, num int, timeout chan bool, closestUpdatedCh chan bool) {
	contacts := sl.GetInactiveContacts(num)
	log.Println("FindNodeCycle inactiveContact:", len(contacts))
	// no inactive contacts so far, just return false
	if contacts == nil {
		closestUpdatedCh <- false
		return
	}

	returnedContactsCh := make(chan ReturnedContacts)
	size := len(contacts)

	for i := 0; i < size; i++ {
		go ka.ReachOutForContacts(returnedContactsCh, contacts[i], id)
	}

	// wait some time before send out next round of cycle
	go func() {
		time.Sleep(300 * time.Millisecond)
		timeout <- true
	}()

	// for each returnedContact, update shortlist,
	// and either mark source active or remove it
	closestUpdated := false
	for i := 0; i < size; i++ {
		res := <- returnedContactsCh
		if res.Contacts == nil {
			// remove non-responding contact from shortlist
			sl.RemoveContact(res.Source)
		} else {
			sl.MarkContactAsActive(res.Source)
			closestUpdated = (closestUpdated || sl.AddContacts(res.Contacts))
		}
	}

	log.Println("FindNodeCycle closestUpdated: ", closestUpdated)
	closestUpdatedCh <- closestUpdated
	return
}

// iteratively find closest k contacts
// TODO: and print out some nice formatted info
func (ka *Kademlia) DoIterativeFindNode(id ID) ([]Contact, error) {
	// initialize an empty ShortList
	var sl ShortList
	sl.Init(k, id)

	// only select alpha number of contacts
	// realSize might be less than alpha
	foundContacts := ka.FindCloseNodes(id, alpha)
	if len(foundContacts) > alpha {
		foundContacts = foundContacts[:alpha]
	}
	realSize := len(foundContacts)
	if realSize == 0 {
		//NOTE: do we still need to print it?
		return nil, &ContactNotFoundError{id, "no contact found in routing table, abort DoIterativeFindNode"}
	}
	log.Println("returning from FindCloseNodes, with ", realSize)
	// add to shortlist
	sl.AddContacts(foundContacts)

	// The sequence of parallel searches is continued until either
	// 1. no node in the sets returned is closer than the closest node already seen or
	// 2. the initiating node has accumulated k probed and known to be active contacts.
	isClosestChangedCh := make(chan bool)
	timeout := make(chan bool)
	go ka.FindNodeCycle(&sl, id, alpha, timeout, isClosestChangedCh)

	for sl.GetInactiveCount() != 0 {
		select {
		case isClosestChanged := <- isClosestChangedCh:
			if isClosestChanged {
				go ka.FindNodeCycle(&sl, id, alpha, timeout, isClosestChangedCh)
			} else {
				numInactive := sl.GetInactiveCount()
				go ka.FindNodeCycle(&sl, id, numInactive, timeout, isClosestChangedCh)
			}
		case <-timeout: // after 300ms, fire the next round of cycle
			go ka.FindNodeCycle(&sl, id, alpha, timeout, isClosestChangedCh)
		}
	}

	res := "\nTotal contacts number: " + strconv.Itoa(len(sl.ContactInfos)) + "\n"
	contacts := make([]Contact, 0)
	for _, coninfo := range sl.ContactInfos {
		con := coninfo.Node
		contacts = append(contacts, con)
		res += "NodeID = " + con.NodeID.AsString() + "\n"
		res += "Host = " + con.Host.String() + "\n"
		res += "Port = " + strconv.Itoa(int(con.Port))
		res += "-------------------------------------\n"
	}

	log.Println(res)

	return contacts, nil
}

func (ka *Kademlia) DoIterativeStore(key ID, value []byte) ([]Contact, error) {
	foundContacts, err := ka.DoIterativeFindNode(key)
 	if err != nil {
 		return nil, err
 	}
	log.Println()
	// send Store RPC to every contact
 	for _, c := range foundContacts {
 		ka.DoStore(&c, key, value)
 	}

 	return foundContacts, nil
}

func (ka *Kademlia) ReachOutForContactsValues(returnedContactsCh chan ReturnedContacts, foundValueCh chan []byte, c Contact, id ID) {
	// acquire local closest contacts
	log.Println("ReachOutForContactsValues...")
	val, contacts, err := ka.DoFindValue(&c, id)
	var res ReturnedContacts
	if err != nil {
		res.Contacts = nil
	} else if val != nil {
		// FIXME return directly
		foundValueCh <- val
		return
	} else {
		// remove itself from ReturnedContacts
		for i, con := range contacts {
			if con.NodeID.Equals(ka.NodeID) {
				contacts = append(contacts[:i], contacts[i+1:]...)
				break
			}
		}

		res.Contacts = contacts
	}
	res.Source = c

	returnedContactsCh <- res
}

// Assume DoFindValue always returnss
func (ka *Kademlia) FindValueCycle(sl *ShortList, id ID, num int, timeout chan bool, closestUpdatedCh chan bool, foundValueCh chan []byte) {
	contacts := sl.GetInactiveContacts(num)
	log.Println("FindNodeCycle inactiveContact:", len(contacts))
	// no inactive contacts so far, just return false
	if contacts == nil {
		closestUpdatedCh <- false
		return
	}

	returnedContactsCh := make(chan ReturnedContacts)
	size := len(contacts)

	for i := 0; i < size; i++ {
		go ka.ReachOutForContactsValues(returnedContactsCh, foundValueCh, contacts[i], id)
	}

	// wait some time before send out next round of cycle
	go func() {
		time.Sleep(300 * time.Millisecond)
		timeout <- true
	}()

	// for each returnedContact, update shortlist,
	// and either mark source active or remove it
	closestUpdated := false
	for i := 0; i < size; i++ {
		res := <-returnedContactsCh
		if res.Contacts == nil {
			// remove non-responding contact from shortlist
			sl.RemoveContact(res.Source)
		} else {
			sl.MarkContactAsActive(res.Source)
			closestUpdated = (closestUpdated || sl.AddContacts(res.Contacts))
		}
	}

	log.Println("FindNodeCycle closestUpdated: ", closestUpdated)
	closestUpdatedCh <- closestUpdated
	return
}

func (ka *Kademlia) DoIterativeFindValue(key ID) (value []byte, err error) {
	// initialize an empty ShortList
	var sl ShortList
	sl.Init(k, key)

	// only select alpha number of contacts
	// realSize might be less than alpha
	foundContacts := ka.FindCloseNodes(key, alpha)
	if len(foundContacts) > alpha {
		foundContacts = foundContacts[:alpha]
	}
	realSize := len(foundContacts)
	if realSize == 0 {
		//NOTE: do we still need to print it?
		return nil, &ContactNotFoundError{key, "contact found in routing table, abort DoIterativeFindNode"}
	}
	log.Println("returning from FindCloseNodes, with ", realSize)
	// add to shortlist
	sl.AddContacts(foundContacts)

	// The sequence of parallel searches is continued until either
	// 1. no node in the sets returned is closer than the closest node already seen or
	// 2. the initiating node has accumulated k probed and known to be active contacts.
	isClosestChangedCh := make(chan bool)
	foundValueCh := make(chan []byte)
	timeout := make(chan bool)

	go ka.FindValueCycle(&sl, key, alpha, timeout, isClosestChangedCh, foundValueCh)

	for sl.GetInactiveCount() != 0 {
		select {
		case valueFound := <-foundValueCh:
			ka.DoStore(sl.ClosestNode, key, valueFound)
			return valueFound, nil
		case isClosestChanged := <-isClosestChangedCh:
			if isClosestChanged {
				go ka.FindValueCycle(&sl, key, alpha, timeout, isClosestChangedCh, foundValueCh)
			} else {
				numInactive := sl.GetInactiveCount()
				go ka.FindValueCycle(&sl, key, numInactive, timeout, isClosestChangedCh, foundValueCh)
			}
		case <-timeout: // after 300ms, fire the next round of cycle
			go ka.FindValueCycle(&sl, key, alpha, timeout, isClosestChangedCh, foundValueCh)
		}
	}

	log.Printf("Fail to find value. Closest node: %s\n", sl.ClosestNode.NodeID.AsString())

	return nil, errors.New(sl.ClosestNode.NodeID.AsString())
}

/**
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 *
 */

// For project 3!
func (ka *Kademlia) Vanish(vdoID ID, data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	// create vdo and sprinkle key shares in its contacts
	vdo = ka.VanishData(vdoID, data, numberKeys, threshold, timeoutSeconds)

	// store the created vdo
	ka.semVdos <- 1
	ka.Vdos[vdoID] = vdo
	<- ka.semVdos
	log.Println("Vanish......, with vdoID:", vdoID.AsString())
	return vdo
}
// // TODO: don't know what is searchKey here, I assume it's the VDOID; could be wrong
// func (ka *Kademlia) Unvanish(searchKey ID) (data []byte) {
// 	//find the vdo in local vdos
// 	ka.semVdos <- 1
// 	vdo, ok := ka.Vdos[searchKey]
// 	<- ka.semVdos
//
// 	if ok != true {
// 		log.Println("Unvanish Error: Failed to find the VDO locally: ", searchKey.AsString())
// 		return nil
// 	}
//
// 	text := ka.UnvanishData(vdo)
// 	return text
// }

// TODO: don't know if this would be useful; this whole assignment is confusing
// just write it down so later we can just copy and paste into right place
func (ka *Kademlia) Unvanish(NodeID ID, vdoID ID) (data []byte) {
	// check if itself has it
	//find the vdo in local vdos
	ka.semVdos <- 1
	vdo, ok := ka.Vdos[vdoID]
	<- ka.semVdos

	if ok == true {
		// has it on this node itself
		data = ka.UnvanishData(vdo)
		if data != nil {
			return data
		}
	}

	// local find node
	foundContacts := ka.FindCloseNodes(NodeID, k)
	for _, contact := range foundContacts {
		if contact.NodeID.Equals(NodeID) {
			// find it and ask it for vdo
			vdo, err := ka.DoGetVDO(&contact, vdoID)
			if err == nil {
				data = ka.UnvanishData(vdo)

				if data != nil {
					return data
				}
			}
		}
	}

	// iterative find node
	contacts, _ := ka.DoIterativeFindNode(NodeID)
	if contacts == nil {
		return nil
	}

	for _, contact := range contacts {
		if contact.NodeID.Equals(NodeID) {
			// find it and ask it for vdo
			vdo, err := ka.DoGetVDO(&contact, vdoID)
			if err == nil {
				data = ka.UnvanishData(vdo)

				if data != nil {
					return data
				}
			}
		}
	}

	// fail
	return nil
}

// TODO: don't know if this would be useful; this whole assignment is confusing
func (ka *Kademlia) DoGetVDO(contact *Contact, vdoID ID) (VanashingDataObject, error) {
	address := contact.Host.String() + ":" + strconv.Itoa(int(contact.Port))
	path := rpc.DefaultRPCPath + strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		return VanashingDataObject{}, errors.New("Failed to dial address: " + address)
	}

	request := new(GetVDORequest)
	request.Sender = ka.SelfContact
	request.VdoID = vdoID
	request.MsgID = NewRandomID()

	var result GetVDOResult
	err = client.Call("KademliaRPC.GetVDO", request, &result)
	if err != nil {
		return VanashingDataObject{}, errors.New("GetVDOError: " + err.Error())
	}

	if result.MsgID.Equals(request.MsgID) {
		vdo := result.VDO
		return vdo, nil
	}

	//MsgID mismatch
	return VanashingDataObject{}, &MsgIDMismatchError{"MsgID mismatch between request and result for DoGetVDO"}
}

// update the input contact in the appropriate kbucket
func (ka *Kademlia) UpdateContact(update *Contact) {
	ka.sem <- 1
	index := ka.FindBucketIndex(update.NodeID)
	bucket := ka.KbucketList[index]
	log.Println("updating contact: ", update)
	err := bucket.Update(*update)
	//bucket is full
	if err != nil {
		// relinquish control because target could ping back
		<-ka.sem
		first := bucket.ContactList[0]
		_, err := ka.DoPing(first.Host, first.Port)
		ka.sem <- 1
		//first contact does not respond, remove it and add update at the tail
		if err != nil {
			bucket.RemoveContact(first.NodeID)
			bucket.AddContact(*update)
		}
	}

	ka.KbucketList[index] = bucket

	// log.Println("After updating, Routing Table:")
	// ka.PrintKbucketList()
	<-ka.sem
}

// find the appropriate kbucket for input id
func (ka *Kademlia) FindBucketIndex(nodeId ID) (index int) {
	prefix := ka.NodeID.Xor(nodeId).PrefixLen()
	if prefix == 160 {
		index = 0
	} else {
		index = 159 - prefix
	}

	return index
}

// find the k closest contacts in the KbucketList
func (ka *Kademlia) FindCloseNodes(nodeID ID, numNodes int) []Contact {
	index := ka.FindBucketIndex(nodeID)

	bucket := ka.KbucketList[index]

	nodes := make([]Contact, 0)
	log.Println("Calling FindCloseNodes by: ", ka.SelfContact.Port)
	ka.sem <- 1
	ka.PrintKbucketList()
	for i := 0; i < len(bucket.ContactList); i++ {
		nodes = append(nodes, bucket.ContactList[i])
	}

	if len(nodes) >= numNodes {
		<-ka.sem
		return nodes
	}

	// the closes bucket is not full, find nodes in other buckets
	// until there are k nodes or all buckets are searched
	var left int = index - 1
	var right int = index + 1

	for { // might need to only return at most k tripels
		if len(nodes) >= numNodes {
			<-ka.sem
			return nodes
		}

		if left <= -1 && right >= bucketsCount {
			// all buckest are searched, just return the nodes
			<-ka.sem
			return nodes
		}

		if right < bucketsCount {
			for _, node := range ka.KbucketList[right].ContactList {
				if node.Host != nil {
					nodes = append(nodes, node)
				}

			}
			right += 1
		}

		if left >= 0 {
			for _, node := range ka.KbucketList[left].ContactList {
				if node.Host != nil {
					nodes = append(nodes, node)
				}

			}
			left -= 1
		}
	}
}

func (ka *Kademlia) StoreKeyVal(key ID, val []byte) {
	copyval := make([]byte, len(val))
	copy(copyval, val)
	// store key, val to table
	ka.semTable <- 1
	ka.Table[CopyID(key)] = copyval
	<-ka.semTable
}
