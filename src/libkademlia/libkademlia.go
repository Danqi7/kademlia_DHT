package libkademlia

// Contains the core kademlia type. In addition to core state, this type serves
// as a receiver for the RPC methods, which is required by that package.

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
)

const (
	alpha = 3
	b     = 8 * IDBytes
	k     = 20
	bucketsCount = 160
)

// assume one goroutine can access whole bucketlist at one time
var sem = make(chan int, 1)

// Kademlia type. You can put whatever state you need in this.
type Kademlia struct {
	NodeID      ID
	SelfContact Contact
	KbucketList []Kbucket
}

func NewKademliaWithId(laddr string, nodeID ID) *Kademlia {
	k := new(Kademlia)
	k.NodeID = nodeID

	// TODO: Initialize other state here as you add functionality.

	//init 160 kbuckets
	k.KbucketList = make([]Kbucket, bucketsCount)
	for i:= 0; i < bucketsCount; i++ {
		k.KbucketList[i].Init(k.NodeID)
	}



	// Set up RPC server
	// NOTE: KademliaRPC is just a wrapper around Kademlia. This type includes
	// the RPC functions.

	s := rpc.NewServer()
	s.Register(&KademliaRPC{k})
	hostname, port, err := net.SplitHostPort(laddr)
	if err != nil {
		return nil
	}
	s.HandleHTTP(rpc.DefaultRPCPath+hostname+port,
		rpc.DefaultDebugPath+hostname+port)

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
	k.SelfContact = Contact{k.NodeID, host, uint16(port_int)}
	return k
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

func (k *Kademlia) FindContact(nodeId ID) (*Contact, error) {
	sem <- 1
	if nodeId == k.SelfContact.NodeID {
		<- sem
		return &k.SelfContact, nil
	}
	// a comprehensive search for all kbuckets
	// maybe a more efficient way to find contact is to search in the closest kbucket
	for i := 0 ; i < len(k.KbucketList); i++ {
		kb := k.KbucketList[i]
		for j := 0; j < len(kb.ContactList); j++ {
			current := kb.ContactList[j]
			if current.NodeID.Equals(nodeId) {
				<- sem
				return &current, nil
			}
		}
	}
	<- sem
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

func (k *Kademlia) DoPing(host net.IP, port uint16) (*Contact, error) {
	address := host.String() + ":" + strconv.Itoa(int(port))
	path := rpc.DefaultRPCPath + "localhost" + strconv.Itoa(int(port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		log.Fatal("Dialing: ", err, address)
	}

	PingMsg := new(PingMessage)
	PingMsg.Sender = k.SelfContact
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
		k.UpdateContact(&update)
		return &update,nil
	}

	//otherwise, no response
	return nil, &NoResponse{"No response from pinged contact"}



}

func (k *Kademlia) DoStore(contact *Contact, key ID, value []byte) error {
	// TODO: Implement
	return &CommandFailed{"Not implemented"}
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

func (k *Kademlia) DoFindNode(contact *Contact, searchKey ID) ([]Contact, error) {
	// TODO: Implement
	address := contact.Host.String() + ":" + strconv.Itoa(int(contact.Port))
	path := rpc.DefaultRPCPath + "localhost" + strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", address, path)
	if err != nil {
		log.Fatal("Dialing: ", err, address)
	}

	request := new(FindNodeRequest)
	request.Sender = *contact
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
		k.UpdateContact(contact)

		// add returned contacts to its kbuckets, don't add the requestor itself
		// might need this, need to double check with fabian
		for _, cont := range result.Nodes {
			if cont.NodeID.Equals(k.NodeID) {
				continue
			}
			k.UpdateContact(&cont)
		}

		return result.Nodes, nil
	}

	//MsgID mismatch
	return nil, &MsgIDMismatchError{"MsgID mismatch between request and result for FindNode"}
}

func (k *Kademlia) DoFindValue(contact *Contact,
	searchKey ID) (value []byte, contacts []Contact, err error) {
	// TODO: Implement
	return nil, nil, &CommandFailed{"Not implemented"}
}

func (k *Kademlia) LocalFindValue(searchKey ID) ([]byte, error) {
	// TODO: Implement
	return []byte(""), &CommandFailed{"Not implemented"}
}

// For project 2!
func (k *Kademlia) DoIterativeFindNode(id ID) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (k *Kademlia) DoIterativeStore(key ID, value []byte) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (k *Kademlia) DoIterativeFindValue(key ID) (value []byte, err error) {
	return nil, &CommandFailed{"Not implemented"}
}

// For project 3!
func (k *Kademlia) Vanish(data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	return
}

func (k *Kademlia) Unvanish(searchKey ID) (data []byte) {
	return nil
}

// update the input contact in the appropriate kbucket
func (k *Kademlia) UpdateContact(update *Contact) {
	sem <- 1
	index := k.FindBucketIndex(update.NodeID)
	bucket := k.KbucketList[index]
	// log.Println("before, NodeID:", k.NodeID)
	// bucket.PrintBucket()
	// log.Println("updating...")
	err := bucket.Update(*update)
	//bucket is full
	if err != nil {
		first := bucket.ContactList[0]
		_, err  := k.DoPing(first.Host, first.Port)
		//first contact does not respond, remove it and add update at the tail
		if err != nil {
			bucket.RemoveContact(first.NodeID)
			bucket.AddContact(*update)
		}
	}
	// log.Println("After")
	// bucket.PrintBucket()

	k.KbucketList[index] = bucket
	<- sem
}

// find the appropriate kbucket for input id
func (k *Kademlia) FindBucketIndex(nodeId ID) (index int){
	prefix := k.NodeID.Xor(nodeId).PrefixLen()
	if prefix == 160 {
		index = 0
	} else {
		index = 159 - prefix
	}

	return index
}

// find the k closest contacts in the KbucketList
func (k *Kademlia) FindCloseNodes(nodeID ID) ([]Contact) {
	index := k.FindBucketIndex(nodeID)

	bucket := k.KbucketList[index]

	nodes := make([]Contact, 0)
	// log.Println("finding close nodes ?????>>>>>>>>>>>", index, len(k.KbucketList))
	sem <- 1
	for i := 0; i < len(bucket.ContactList); i++ {
		nodes = append(nodes, bucket.ContactList[i])
	}

	if len(nodes) == 20 {
		<- sem
		return nodes
	}

	// for _, val := range k.KbucketList {
	// 	val.PrintBucket()
	// }
	// log.Println("finding close nodes ?????>>>>>>>>>>>", k.NodeID)

	// the closes bucket is not full, find nodes in other buckets
	// until there are k nodes or all buckets are searched
	left := index-1
	right := index+1

	for { // might need to only return at most k tripels

		if len(nodes) == 20 {
			<- sem
			return nodes
		}

		if left == -1 && right == bucketsCount {
			// all buckest are searched, just return the nodes
			<- sem
			return nodes
		}

		if right < bucketsCount {
			for _, node := range k.KbucketList[right].ContactList {
				if node.Host != nil {
					nodes = append(nodes, node)
				}

			}
			right += 1
		}

		if left >= 0 {
			for _, node := range k.KbucketList[left].ContactList {
				if node.Host != nil {
					nodes = append(nodes, node)
				}

			}
			left -= 1
		}
	}
}
