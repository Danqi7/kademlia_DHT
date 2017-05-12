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
	NodeID      ID
	SelfContact Contact
	KbucketList []Kbucket
	Table       map[ID][]byte
	sem         chan int
	semTable    chan int
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

	// init ka.semaphores
	ka.sem = make(chan int, 1)
	ka.semTable = make(chan int, 1)

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
		log.Fatal("Dialing: ", err, address)
	}

	PingMsg := new(PingMessage)
	PingMsg.Sender = ka.SelfContact
	PingMsg.MsgID = NewRandomID()

	//Dial RPC
	var PongMsg PongMessage
	err = client.Call("KademliaRPC.Ping", PingMsg, &PongMsg)
	if err != nil {
		log.Fatal("RPC Ping: ", err)
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
		log.Fatal("Dialing: ", err, address)
	}

	request := new(StoreRequest)
	request.MsgID = NewRandomID()
	request.Sender = ka.SelfContact
	request.Key = CopyID(key)
	request.Value = value

	var result StoreResult
	err = client.Call("KademliaRPC.Store", request, &result)
	if err != nil {
		log.Fatal("DoStore: ", err)
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
		log.Fatal("FindNode: ", err)
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
		log.Fatal("Dialing: ", err, address)
	}

	request := new(FindValueRequest)
	request.MsgID = NewRandomID()
	request.Sender = ka.SelfContact
	request.Key = CopyID(searchKey)

	var result FindValueResult
	err = client.Call("KademliaRPC.FindValue", request, &result)
	if err != nil {
		log.Fatal("FindValue: ", err)
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

type ReturnedContact struct {
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
func (ka *Kademlia) ReachOutForContact(channel chan ReturnedContact, c Contact, id ID) {
	// acquire contacts
	contacts, err := ka.DoFindNode(&c, id)
	var res ReturnedContact
	if err != nil {
		res.Contacts = nil
	} else {
		res.Contacts = contacts
	}
	res.Source = c
	channel <- res
}

func (ka *Kademlia) IterativeFindSingleCycle(sl *ShortList, id ID, a int) bool {

	/////// DEBUG ->
	log.Printf("IterativeFindSingleCycle: starting...\n")
	/////// <- DEBUG

	nextAlpha := sl.GetNextAlphaInactive(a)
	if nextAlpha == nil {
		/////// DEBUG ->
		log.Printf("IterativeFindSingleCycle: no more inactive; returning\n")
		/////// <- DEBUG
		return false
	}
	returnedContacts := make(chan ReturnedContact)
	traverseEnd := len(nextAlpha)
	/////// DEBUG ->
	log.Printf("IterativeFindSingleCycle: going to call %d contact to get more contacts\n", traverseEnd)
	/////// <- DEBUG
	for i := 0; i < traverseEnd; i++ {
		go ka.ReachOutForContact(returnedContacts, nextAlpha[i], id)
		// wait for 300 milliseconds
		time.Sleep(300 * time.Millisecond)
	}
	// The node then fills the shortlist with contacts from the replies received
	var rc ReturnedContact
	var isCycleInchingCloser bool = false
	for i := 0; i < traverseEnd; i++ {
		rc = <-returnedContacts
		/////// DEBUG ->
		log.Printf("IterativeFindSingleCycle: processing returned contact\n")
		/////// <- DEBUG
		if rc.Contacts == nil {
			/////// DEBUG ->
			log.Printf("IterativeFindSingleCycle: contact did not respond, removing (ID: %s)\n", rc.Source.NodeID.AsString())
			/////// <- DEBUG
			sl.RemoveContact(rc.Source)
		} else {
			/////// DEBUG ->
			log.Printf("IterativeFindSingleCycle: contact responded; marking as active (ID: %s). Returned %d contacts \n", rc.Source.NodeID.AsString(), len(rc.Contacts))
			/////// <- DEBUG
			sl.MarkContactAsActive(rc.Source)
			isCycleInchingCloser = (isCycleInchingCloser || sl.AddAlpha(rc.Contacts, len(rc.Contacts)))
		}
	}
	/////// DEBUG ->
	log.Printf("IterativeFindSingleCycle: about to return... are we closer to key ID? %t; Length of Contact list: %d\n", isCycleInchingCloser, len(sl.Contacts))
	/////// <- DEBUG
	return isCycleInchingCloser
}

func (ka *Kademlia) DoIterativeFindNode(id ID) ([]Contact, error) {

	var err error
	/////// DEBUG ->
	log.Printf("DoIterativeFindNode: START looking for: %s\n", id.AsString())
	/////// <- DEBUG

	// initialize an empty ShortList
	var sl ShortList
	sl.Init(k)

	// ITERATION
	// select 3 (alpha) contacts
	foundContacts := ka.FindCloseNodes(id, alpha)
	/////// DEBUG ->
	log.Printf("DoIterativeFindNode: first run; closest contacts: %d\n", len(foundContacts))
	for i := 0; i < len(foundContacts); i++ {
		log.Printf("- Returned ID: %s\n", foundContacts[i].NodeID.AsString())
	}
	/////// <- DEBUG

	// keep track of the closest contact of the 3, and add the 3 to the short list
	if !sl.AddAlpha(foundContacts, alpha) {
		/////// DEBUG ->
		log.Printf("DoIterativeFindNode: Cannot find anything... quitting...\n")
		/////// <- DEBUG
		return nil, err
	}

	// The sequence of parallel searches is continued until either
	// 1. no node in the sets returned is closer than the closest node already seen or
	// 2. the initiating node has accumulated k probed and known to be active contacts.
	var isClosestChanged bool = true
	for isClosestChanged == true && (len(sl.Contacts) < k || sl.NumInactive > 0) {

		/////// DEBUG ->
		log.Printf("DoIterativeFindNode: enter another iteration.\n")
		for i := 0; i < len(foundContacts); i++ {
			log.Printf("- Currently available ID: %s\n", foundContacts[i].NodeID.AsString())
		}
		/////// <- DEBUG

		// sends parallel, asynchronous FIND_* RPCs to the alpha contacts in the shortlist
		isClosestChanged = ka.IterativeFindSingleCycle(&sl, id, alpha)

		// if closestNode is unchanged,
		// then the initiating node sends a FIND_* RPC to each of the k closest nodes
		// that it has not already queried.
		if !isClosestChanged {
			/////// DEBUG ->
			log.Printf("DoIterativeFindNode: closest did not change... entering cycle end spam\n")
			/////// <- DEBUG
			isClosestChanged = ka.IterativeFindSingleCycle(&sl, id, sl.NumInactive)
		}
	}

	/////// DEBUG ->
	log.Printf("DoIterativeFindNode: Grand Return\n")
	for i := 0; i < len(sl.Contacts); i++ {
		log.Printf("- Contact ID: %s\n", sl.Contacts[i].NodeID.AsString())
	}
	/////// <- DEBUG

	return sl.Contacts, nil
}

func (ka *Kademlia) DoIterativeStore(key ID, value []byte) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (ka *Kademlia) DoIterativeFindValue(key ID) (value []byte, err error) {
	return nil, &CommandFailed{"Not implemented"}
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
func (ka *Kademlia) Vanish(data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	return
}

func (ka *Kademlia) Unvanish(searchKey ID) (data []byte) {
	return nil
}

// update the input contact in the appropriate kbucket
func (ka *Kademlia) UpdateContact(update *Contact) {
	ka.sem <- 1
	index := ka.FindBucketIndex(update.NodeID)
	bucket := ka.KbucketList[index]
	// log.Println("before, NodeID:", ka.NodeID)
	// bucket.PrintBucket()
	// log.Println("updating...")
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
	// log.Println("After")
	// bucket.PrintBucket()

	ka.KbucketList[index] = bucket
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
	// log.Println("finding close nodes ?????>>>>>>>>>>>", index, len(ka.KbucketList))
	ka.sem <- 1
	for i := 0; i < len(bucket.ContactList); i++ {
		nodes = append(nodes, bucket.ContactList[i])
	}

	if len(nodes) >= numNodes {
		<-ka.sem
		return nodes
	}

	// for _, val := range ka.KbucketList {
	// 	val.PrintBucket()
	// }
	// log.Println("finding close nodes ?????>>>>>>>>>>>", ka.NodeID)

	// the closes bucket is not full, find nodes in other buckets
	// until there are k nodes or all buckets are searched
	left := index - 1
	right := index + 1

	for { // might need to only return at most k tripels

		if len(nodes) >= numNodes {
			<-ka.sem
			return nodes
		}

		if left == -1 && right == bucketsCount {
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
