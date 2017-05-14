package libkademlia

import (
	//"fmt"
    "log"
	//"net"
	"testing"
    "strconv"
)

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
                t.Error("instacen1, localhost:7000, trying to find localhost:7002 should fail")
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
