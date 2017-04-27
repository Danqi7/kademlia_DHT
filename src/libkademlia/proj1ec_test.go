package libkademlia

import (
	"fmt"
	"net"
	"testing"
)

const (
	instance_count      = 21
	localhostAndPartIP1 = "localhost:71%02d"
	localhostAndPartIP2 = "localhost:72%02d"
	localhost           = "127.0.0.1"
	portBase1           = 7100
	portBase2           = 7200
)

func TestFullBucket(t *testing.T) {

	var ids [instance_count]ID
	var ks [instance_count](*Kademlia)

	// make 21 instances in the same bucket
	oneID := NewRandomID()
	oneID[18] = byte(0x0)
	for i := 0; i < instance_count; i++ {
		oneID[19] = byte(i)
		ids[i] = CopyID(oneID)
		ks[i] = NewKademliaWithId(fmt.Sprintf(localhostAndPartIP1, i), ids[i])
		// t.Logf("Instance k%02d: %s; %s", i, fmt.Sprintf(localhostAndPartIP1,   i), ids[i].AsString())
	}

	// 21st Kademlia
	oneID[19] = byte(instance_count)
	oneID[18] = byte(0x1)
	idTester := CopyID(oneID)
	addrTester := fmt.Sprintf(localhostAndPartIP1, instance_count)
	kTester := NewKademliaWithId(addrTester, idTester)

	// t.Logf("kTester NodeID: %s; k%02d NodeID: %s; XOR: %s; distance: %d", kTester.NodeID.AsString(), instance_count-1, ids[instance_count-1].AsString(), kTester.NodeID.Xor(ids[instance_count-1]).AsString(), kTester.NodeID.Xor(ids[instance_count-1]).PrefixLen())

	// reach everyone in order; that should give us 20 nodes on the list, and the last one not on the list
	parseIP := net.ParseIP(localhost)
	var e error
	for i := 0; i < instance_count; i++ {
		_, e = kTester.DoPing(parseIP, uint16(portBase1+i))
		if e != nil {
			t.Error(fmt.Sprintf("kTester Could not ping k%02d\n", i))
		}
	}

	// try to have kTester to find 0-19
	for i := 0; i < instance_count-1; i++ {
		_, e := kTester.FindContact(ids[i])
		if e != nil {
			t.Error(fmt.Sprintf("Cannot find k%02d\n", i))
		}
	}

	// make sure that kTester cannot find 20
	_, e = kTester.FindContact(ids[instance_count-1])
	if e == nil {
		t.Error(fmt.Sprintf("Somehow kTester also stored k%02d\n", instance_count-1))
	}

	return
}

func TestNewlyResolvedNodeIsAlwaysAtTailOfBucket(t *testing.T) {

	var ids [instance_count]ID
	var ks [instance_count](*Kademlia)

	// make 21 instances in the same bucket
	oneID := NewRandomID()
	oneID[18] = byte(0x0)
	for i := 0; i < instance_count; i++ {
		oneID[19] = byte(i)
		ids[i] = CopyID(oneID)
		ks[i] = NewKademliaWithId(fmt.Sprintf(localhostAndPartIP2, i), ids[i])
		// t.Logf("Instance k%02d: %s; %s", i, fmt.Sprintf(localhostAndPartIP1,   i), ids[i].AsString())
	}

	// 21st Kademlia
	oneID[19] = byte(instance_count)
	oneID[18] = byte(0x1)
	idTester := CopyID(oneID)
	addrTester := fmt.Sprintf(localhostAndPartIP2, instance_count)
	kTester := NewKademliaWithId(addrTester, idTester)

	// reach everyone
	parseIP := net.ParseIP(localhost)
	var e error
	for i := 0; i < instance_count; i++ {
		_, e = kTester.DoPing(parseIP, uint16(portBase1+i))
		if e != nil {
			t.Error(fmt.Sprintf("kTester Could not ping k%02d\n", i))
		}
	}

	// ping 0
	_, e = kTester.DoPing(parseIP, uint16(portBase2))
	if e != nil {
		t.Error(fmt.Sprintf("kTester Could not ping k%02d\n", 0))
	}

	// check if 0 is the newest
	bucketIndex := kTester.FindBucketIndex(ids[0])

	l := kTester.KbucketList[bucketIndex].ContactList
	if !l[len(l)-1].NodeID.Equals(ks[0].NodeID) {
		t.Error(fmt.Sprintf("ks[0] is not last on kTester's bucket"))
	}
}
