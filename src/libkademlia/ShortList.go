package libkademlia

import (
	"errors"
	"log"
	"sort"
)

type ContactInfo struct {
	Node 	Contact
	Status	bool // for isActive
}

type ShortList struct {
	Contacts    		[]Contact
	ActiveList  		[]bool
	TargetID    		ID
	ClosestNode     	*Contact
	FirstInactivePos	int // position of the first not yet probed node, if exists
	Capacity    		int
	sem         		chan int
}

func (sl *ShortList) Init(k int, target ID) {
	sl.Contacts = make([]Contact, 0, k)
	sl.ActiveList = make([]bool, 0, k)
	sl.TargetID = target
	sl.ClosestNode = nil
	sl.FirstInactivePos = 0
	sl.Capacity = k
	sl.sem = make(chan int, 1)
}

// update ClosestNode if the input node is closer
// return true if ClosestNode is updated
func (sl *ShortList) UpdateClosest(contact Contact) bool {
	if sl.ClosestNode == nil {
		sl.ClosestNode = &contact
		return true
	}
	closestDistance := sl.TargetID.Xor(sl.ClosestNode.NodeID)
	currentDistance := sl.TargetID.Xor(contact.NodeID)

	if currentDistance.Less(closestDistance) {
		sl.ClosestNode = &contact
		return true
	}

	return false
}


// add input contact to ShortList if
// and ShortList doesn't contain the input contact
func (sl *ShortList) AddContact(contact Contact) bool {

	// check if the contact is already in the ShortList
	for _, node := range sl.Contacts {
		if contact.NodeID.Equals(node.NodeID) {
			return false
		}
	}

	// check if ShortList is full
	if len(sl.Contacts) >= sl.Capacity {
		return false
	}

	// add the new contact to the ShortList
	// mark it as inactive
	sl.Contacts = append(sl.Contacts, contact)
	sl.ActiveList = append(sl.ActiveList, false)

	// update the closestNode if possible, now assume update everytime new contact added
	isClosestChanged := sl.UpdateClosest(contact)

	log.Println("AddContact closestUpdated: ", isClosestChanged)

	return isClosestChanged
}

// Replace inactive contact at pos with newContact,
// Assume newContact is closer than the inactive contact
func (sl *ShortList) ReplaceInactiveContact(pos int, contact Contact) bool {
	// check if the contact is already in the ShortList
	// if already in, simply return
	for _, node := range sl.Contacts {
		if contact.NodeID.Equals(node.NodeID) {
			return false
		}
	}

	// try to replace active contact in shortlist, error!
	if sl.ActiveList[pos] {
		log.Fatal("Trying to replace active contact in shortlist, active contact: ", sl.Contacts[pos])
	}

	// replace
	sl.Contacts[pos] = contact
	// update the closestNode if possible, now assume update everytime new contact added
	isClosestChanged := sl.UpdateClosest(contact)

	return isClosestChanged
}

// Bulk-add input contacts; input contacts are already sorted
// Need to sort inactive ShortList.Contacts first
func (sl *ShortList) AddContacts(contacts []Contact) bool {
	var isClosestChanged bool = false
	sl.sem <- 1
	size := len(sl.Contacts)
	index := 0
	i := sl.FirstInactivePos

	// sort input contacts by distance
	sort.Slice(contacts, func(i, j int) bool {
		c1 := contacts[i]
		c2 := contacts[j]

		d1 := sl.TargetID.Xor(c1.NodeID)
		d2 := sl.TargetID.Xor(c2.NodeID)

		return d1.Less(d2)
	})

	// sort inactive contacts in ShortList by distance
	// tips: changing sub-slice changes the orginal slice
	inactive := sl.Contacts[sl.FirstInactivePos:]
	sort.Slice(inactive, func (i, j int) bool {
		c1 := inactive[i]
		c2 := inactive[j]

		d1 := sl.TargetID.Xor(c1.NodeID)
		d2 := sl.TargetID.Xor(c2.NodeID)

		return d1.Less(d2)
	})

	// compare input contacts with inactive contacts
	// replace inactive contacts if input contact is closer
	for ; i < size && index < len(contacts); {
		inactiveContact := sl.Contacts[i]
		contact := contacts[index]
		inactiveDist := sl.TargetID.Xor(inactiveContact.NodeID)
		dist := sl.TargetID.Xor(contact.NodeID)

		// if closer, replace the inactiveContact with the current input contact
		if dist.Less(inactiveDist) {
			isClosestChanged = sl.ReplaceInactiveContact(i, contact)
			i += 1
			index += 1
		} else {
			// try replace the next inactive node, which are farther than ith inactive
			i += 1
		}
	}

	// still has spot in shortlist
	for size < sl.Capacity && index < len(contacts) {
		// add rest input contacts，if any
		isClosestChanged = sl.AddContact(contacts[index])
		index += 1
		size += 1
	}

	log.Println("AddContacts finished: ")
	<- sl.sem
	return isClosestChanged
}

// add input contacts to shortlist
// return true if closestNode is changed
// func (sl *ShortList) AddAlpha(contacts []Contact, alpha int) bool {
// 	size := len(contacts)
// 	if len(contacts) == 0 {
// 		return false
// 	}
// 	if size > alpha {
// 		size = a
// 	}
//
// 	isClosestChanged := false
// 	for i := 0; i < size; i++ {
// 		// add the contact to the short list
// 		isClosestChanged = (sl.Add(contacts[i]) || isClosestChanged)
// 	}
// 	return isClosestChanged
// }

// Remove inactive contact from ShortList
// Return error if contact is not found in ShortList or trying to remove active contact
func (sl *ShortList) RemoveContact(contact Contact) error {
	sl.sem <- 1
	size := len(sl.Contacts)
	for i := 0; i < size; i++ {
		if contact.NodeID.Equals(sl.Contacts[i].NodeID) {
			if sl.ActiveList[i] == true {
				errors.New("Trying to remove active ID: " + contact.NodeID.AsString())
			}
			sl.Contacts = append(sl.Contacts[:i], sl.Contacts[i+1:]...)
			sl.ActiveList = append(sl.ActiveList[:i], sl.ActiveList[i+1:]...)
			<-sl.sem
			return nil
		}
	}
	<-sl.sem
	return errors.New("ID " + contact.NodeID.AsString() + " not found among ShortList contacts (RemoveContact)")
}

func (sl *ShortList) GetInactiveContacts(num int) []Contact {
	sl.sem <- 1

	contacts := make([]Contact, 0)
	for i := sl.FirstInactivePos; i < len(sl.Contacts) && len(contacts) < num; i++ {
		contacts = append(contacts, sl.Contacts[i])
	}

	<-sl.sem
	return contacts
}

func (sl *ShortList) MarkContactAsActive(c Contact) error {
	sl.sem <- 1

	for i := 0; i < len(sl.Contacts); i++ {
		if sl.Contacts[i].NodeID.Equals(c.NodeID) {
			if sl.ActiveList[i] == true {
				log.Println("Trying to mark active contact as active; might be a parallel problem ...")
			} else {
				sl.ActiveList[i] = true
				sl.FirstInactivePos += 1
			}

			<-sl.sem
			return nil
		}
	}

	<-sl.sem
	return errors.New("ID " + c.NodeID.AsString() + " not found among ShortList contacts (MarkContactAsActive)")
}

// Get the number of inactive contacts count in ShortList
func (sl *ShortList) GetInactiveCount() int {
	sl.sem <- 1

	count := len(sl.Contacts) - sl.FirstInactivePos

	<-sl.sem
	return count
}
