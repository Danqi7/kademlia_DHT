package libkademlia

// Contains definitions mirroring the Kademlia spec. You will need to stick
// strictly to these to be compatible with the reference implementation and
// other groups' code.

import (
	"net"
)

type KademliaRPC struct {
	kademlia *Kademlia
}

// Host identification.
type Contact struct {
	NodeID ID
	Host   net.IP
	Port   uint16
}

///////////////////////////////////////////////////////////////////////////////
// PING
///////////////////////////////////////////////////////////////////////////////
type PingMessage struct {
	Sender Contact
	MsgID  ID
}

type PongMessage struct {
	MsgID  ID
	Sender Contact
}

func (ka *KademliaRPC) Ping(ping PingMessage, pong *PongMessage) error {
	pong.MsgID = CopyID(ping.MsgID)
	// Specify the sender
	pong.Sender = ka.kademlia.SelfContact
	// Update contact, etc
	sender := ping.Sender
	ka.kademlia.UpdateContact(&sender)

	return nil
}

///////////////////////////////////////////////////////////////////////////////
// STORE
///////////////////////////////////////////////////////////////////////////////
type StoreRequest struct {
	Sender Contact
	MsgID  ID
	Key    ID
	Value  []byte
}

type StoreResult struct {
	MsgID ID
	Err   error
}

func (ka *KademliaRPC) Store(req StoreRequest, res *StoreResult) error {
	ka.kademlia.StoreKeyVal(req.Key, req.Value)

	res.MsgID = CopyID(req.MsgID)
	res.Err = nil

	// update contact in bucket
	ka.kademlia.UpdateContact(&req.Sender)

	return nil
}

///////////////////////////////////////////////////////////////////////////////
// FIND_NODE
///////////////////////////////////////////////////////////////////////////////
type FindNodeRequest struct {
	Sender Contact
	MsgID  ID
	NodeID ID
}

type FindNodeResult struct {
	MsgID ID
	Nodes []Contact
	Err   error
}

func (ka *KademliaRPC) FindNode(req FindNodeRequest, res *FindNodeResult) error {
	res.MsgID = CopyID(req.MsgID)
	res.Nodes = ka.kademlia.FindCloseNodes(req.NodeID, k)
	res.Err = nil

	//update sender in the kbucket
	ka.kademlia.UpdateContact(&req.Sender)

	return nil
}

///////////////////////////////////////////////////////////////////////////////
// FIND_VALUE
///////////////////////////////////////////////////////////////////////////////
type FindValueRequest struct {
	Sender Contact
	MsgID  ID
	Key    ID
}

// If Value is nil, it should be ignored, and Nodes means the same as in a
// FindNodeResult.
type FindValueResult struct {
	MsgID ID
	Value []byte
	Nodes []Contact
	Err   error
}

func (ka *KademliaRPC) FindValue(req FindValueRequest, res *FindValueResult) error {
	// TODO: Implement.
	val, err := ka.kademlia.LocalFindValue(req.Key)
	// no val with this key, find close nodes instead
	if err != nil {
		res.Nodes = ka.kademlia.FindCloseNodes(req.Sender.NodeID, k)
	} else {
		// val found
		copyval := make([]byte, len(val))
		copy(copyval, val)
		res.Value = copyval
	}

	res.MsgID = CopyID(req.MsgID)
	res.Err = nil

	// update the contact in bucket
	ka.kademlia.UpdateContact(&req.Sender)

	return nil
}

// For Project 3

type GetVDORequest struct {
	Sender Contact
	VdoID  ID
	MsgID  ID
}

type GetVDOResult struct {
	MsgID ID
	VDO   VanashingDataObject
}

func (ka *KademliaRPC) GetVDO(req GetVDORequest, res *GetVDOResult) error {
	// TODO: Implement.
	return nil
}
