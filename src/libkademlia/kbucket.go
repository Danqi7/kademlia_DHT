package libkademlia

import (
	"fmt"
	"log"
)

type Kbucket struct {
	NodeID ID
	ContactList []Contact
}

//var sem = make(chan int, 1) //only one goroutine is allowed to read/write kbuckets

// init a kbucket with empty contact list with capcacity k
func (kb *Kbucket) Init(nodeID ID) {
	kb.ContactList = make([]Contact, 0, k)
	kb.NodeID = nodeID
}

type KbucketFullError struct {
	msg string
}

func (e *KbucketFullError) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (kb *Kbucket) PrintBucket() {
	for i:= 0; i < len(kb.ContactList); i++ {
		log.Println(kb.ContactList[i])
	}
}

// update kbucket whenever receives msg from another node so recently-seen
// node is moved to the tail
func (kb *Kbucket) Update(updated Contact) error {
	exists, _ := kb.ContactExists(updated)

	if kb.NodeID.Equals(updated.NodeID) {
		//no need to update self
		return nil
	}

	// if exists, move the contact to the tail of the list
	if exists {
		//remove the contact
		success := kb.RemoveContact(updated.NodeID)
		if !success {
			log.Panic("trying to remove contact that's not in the kbucket, might be a racing condition.")
		}

		// add contact at the tail
		kb.AddContact(updated)

		return nil
	} else if len(kb.ContactList) < k {
		// if kbucket is not full, add contact at the tail
		kb.AddContact(updated)

		return nil
	} else {
		// if kbucket is full, ping first node in the list
		// if no response, remove the first node and add the contact at the tail
		// if firt node responds,  move the first node to the tail of the list
		return &KbucketFullError{"kbucket is full, need to ping the first contact"}
	}


}


// check if input contact is in the kbucket
// returns index if exits
func (kb *Kbucket) ContactExists(cont Contact) (exists bool, index int) {
	for i := 0; i < len(kb.ContactList); i++ {
		if kb.ContactList[i].NodeID.Equals(cont.NodeID) {
			exists = true
			index = i
			return
		}
	}

	exists = false
	index = -1
	return
}


// remove contact with NodeID id, return true if succeed
func (kb *Kbucket) RemoveContact(id ID) bool {
	for i := 0; i < len(kb.ContactList); i++ {
		if kb.ContactList[i].NodeID.Equals(id) {
			oldList := kb.ContactList
			newList := append(oldList[:i], oldList[i+1:]...)
			kb.ContactList = newList

			return true
		}
	}

	return false
}


// add cont to the tail of the kbucket, assume empty seat at the tail
func (kb *Kbucket) AddContact(cont Contact) bool {
	newCont := new(Contact)
	newCont.NodeID = CopyID(cont.NodeID)
	newCont.Host = cont.Host
	newCont.Port = cont.Port

	kb.ContactList = append(kb.ContactList, cont)

	return true
}
