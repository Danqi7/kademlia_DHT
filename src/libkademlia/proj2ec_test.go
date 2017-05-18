package libkademlia

import (
	//"fmt"
    "log"
	//"net"
    "bytes"
	"testing"
    "strconv"
)

/**
 * Explanation:
 * This function tests DoIterativeFindNode; make instance1 only knows instance2
 * and instance2 knows everyone in the graph so instance1 should fail to find instance3
 * when uses local FindNode, but instance1 should be able to find instance3 by using DoIterativeFindNode
 * Here A is instance1, B is instance2
 */
func TestIterativeFindNode(t *testing.T) {
    // tree structure;
	// A->B->tree
	/*
	         C
	      /
	  A-B -- D
	      \
	         E
	*/
    log.Println("=================TestIterativeFindNode==================")
    instance1 := NewKademlia("localhost:7000")
    instance2 := NewKademlia("localhost:7001")
    host_number1, port_number1, _ := StringToIpPort("localhost:7000")
    instance2.DoPing(host_number1, port_number1)

    instance3 := NewKademlia("localhost:7002")
    host_number3, port_number3, _ := StringToIpPort("localhost:7002")
    instance2.DoPing(host_number3, port_number3)

    tree_node := make([]*Kademlia, 10)
	for i := 0; i < 10; i++ {
		address := "localhost:" + strconv.Itoa(7003+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}

    // instacen1, localhost:7000, trying to find localhost:7002 should fail
    contacts := instance1.FindCloseNodes(instance3.NodeID, 20)
    if len(contacts) == 0 {
        t.Error("localhost:7000 should at least have 7001")
    } else {
        for _, c := range contacts {
            if c.NodeID.Equals(instance3.NodeID) {
                t.Error("instacen1, localhost:7000, trying to find instance3, localhost:7002, should fail")
            }
        }
    }

    // now use iterativeFindNode
    contacts, _ = instance1.DoIterativeFindNode(instance3.NodeID)
    if contacts == nil {
        t.Error("iterativeFindNode has found zero contacts")
    }

    return
}

/**
 * Explanation:
 * This function tests DoIterativeStore; make instance1 only knows instance2
 * and instance2 knows everyone in the graph; everyone should fail to find the key & value
 * before instance1 do the iterativeStore; after the iterativeStore, the number of nodes that store
 * the key & value should equal to the number of nodes that iterativeFindNode has found
 * Here A is instance1, B is instance2
 */
func TestIterativeStore(t *testing.T) {
    // tree structure;
    // A->B->tree
    /*
             C
          /
      A-B -- D
          \
             E
    */
    instance1 := NewKademlia("localhost:7013")
    instance2 := NewKademlia("localhost:7014")
    host_number1, port_number1, _ := StringToIpPort("localhost:7013")
    instance2.DoPing(host_number1, port_number1)

    tree_node := make([]*Kademlia, 5)
	for i := 0; i < 5; i++ {
		address := "localhost:" + strconv.Itoa(7015+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}

    key := NewRandomID()
    value := []byte("Hello World")

    // LocalFindValue on those nodes should fail, producing an not-found error
    for _, node := range tree_node {
        _, err := node.LocalFindValue(key)
        if err == nil {
            t.Error("LocalFindValue on nodes should fail before the iterativeStore")
        }
    }
    contacts, err := instance1.DoIterativeStore(key, value)

    if err != nil {
        t.Error("DoIterativeStore error" + err.Error())
    }

    // the number of foundContacts should equal to the numebr
    // of contacts that can find the value after the iterativeStore
    cnt := 0
    storedValue, err := instance2.LocalFindValue(key)
    if err == nil {
        if !bytes.Equal(storedValue, value) {
    		t.Error("Stored value did not match found value")
    	}
        cnt += 1
    }

    for _, node := range tree_node {
        storedValue, err = node.LocalFindValue(key)
        if err == nil {
            if !bytes.Equal(storedValue, value) {
        		t.Error("Stored value did not match found value")
        	}
            cnt += 1
        }
    }

    if cnt != len(contacts) {
        t.Error("Every found contact should store the value")
    }

    if cnt <= 0 {
        t.Error("At least one contact should have stored the key & value")
    }

    return
}

/**
 * Explanation:
 * This function tests DoIterativeFindValue; make instance1 only knows instance2
 * and instance2 knows everyone in the graph; Instance1 should not be able to find
 * the key & value locallly but should be able to find the key & value when doing iterativeFindValue
 * Here A is instance1, B is instance2
 */
func TestIterativeFindValue(t *testing.T) {
    // tree structure;
    // A->B->tree
    /*
             C
          /
      A-B -- D
          \
             E
    */
    log.Println("=================TestIterativeFindValue==================")
    instance1 := NewKademlia("localhost:7020")
    instance2 := NewKademlia("localhost:7021")
    instance3 := NewKademlia("localhost:7022")
    host_number1, port_number1, _ := StringToIpPort("localhost:7020")
    instance2.DoPing(host_number1, port_number1)
    host_number3, port_number3, _ := StringToIpPort("localhost:7022")
    instance2.DoPing(host_number3, port_number3)

    tree_node := make([]*Kademlia, 5)
	for i := 0; i < 5; i++ {
		address := "localhost:" + strconv.Itoa(7023+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}

    // store key & value in instance3
    key := NewRandomID()
    value := []byte("Hello World")
    contact3, err := instance2.FindContact(instance3.NodeID)
	if err != nil {
		t.Error("Instance 3's contact not found in Instance 2's contact list")
		return
	}
    err = instance2.DoStore(contact3, key, value)
	if err != nil {
		t.Error("Could not store value")
	}

    // instance1 FindValue locallly should fail
    _, err = instance1.LocalFindValue(key)
    if err == nil {
        t.Error("Instance1 should not be able to find the key locallly")
    }

    // iterativeFindValue should succeed
    res, err := instance1.DoIterativeFindValue(key)
    if err != nil {
        t.Error("Instance1 should be able to find the key by using iterativeFindValue")
    }

    if !bytes.Equal(res, value) {
        t.Error("found value did not match stored value")
    }

    return

}
