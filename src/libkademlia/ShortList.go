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
	ContactInfos		[]ContactInfo
	TargetID    		ID
	ClosestNode     	*Contact
	Capacity    		int
	sem         		chan int
}

func (sl *ShortList) Init(k int, target ID) {
	sl.ContactInfos = make([]ContactInfo, 0, k)
	sl.TargetID = target
	sl.ClosestNode = nil
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


// Add input contact to ShortList if ShortList is not full
// and ShortList doesn't contain the input contact
func (sl *ShortList) AddContact(contact Contact) bool {

	// check if the contact is already in the ShortList
	for _, coninfo := range sl.ContactInfos {
		if contact.NodeID.Equals(coninfo.Node.NodeID) {
			return false
		}
	}

	// check if ShortList is full
	if len(sl.ContactInfos) >= sl.Capacity {
		return false
	}

	// add the new contact to the ShortList
	// mark it as inactive
	contactInfo := new(ContactInfo)
	contactInfo.Node = contact
	contactInfo.Status = false

	sl.ContactInfos = append(sl.ContactInfos, *contactInfo)

	// update the closestNode if possible, now assume update everytime new contact added
	isClosestChanged := sl.UpdateClosest(contact)

	log.Println("AddContact closestUpdated: ", isClosestChanged)

	return isClosestChanged
}

// Replace inactive contact at pos with newContact,
// Assume newContact is closer than the inactive contact
func (sl *ShortList) ReplaceInactiveContact(pos int, contact Contact) bool {

	// check if the contact is already in the ShortList
	for _, coninfo := range sl.ContactInfos {
		if contact.NodeID.Equals(coninfo.Node.NodeID) {
			return false
		}
	}

	// try to replace active contact in shortlist, error!
	if sl.ContactInfos[pos].Status == true {
		log.Fatal("Trying to replace active contact in shortlist, active contact: ", sl.ContactInfos[pos])
	}

	// replace
	sl.ContactInfos[pos].Node = contact
	// update the closestNode if possible, now assume update everytime new contact added
	isClosestChanged := sl.UpdateClosest(contact)

	return isClosestChanged
}

// Bulk-add input contacts; Need to sort contacts
// so that replaced contact will not be re-replaced by a closer input node
func (sl *ShortList) AddContacts(contacts []Contact) bool {
	var isClosestChanged bool = false
	sl.sem <- 1

	// sort input contacts by distance
	// the closest contact at first
	sort.Slice(contacts, func(i, j int) bool {
		c1 := contacts[i]
		c2 := contacts[j]

		d1 := sl.TargetID.Xor(c1.NodeID)
		d2 := sl.TargetID.Xor(c2.NodeID)

		return d1.Less(d2)
	})

	// sort contacts in ShortList by distance
	sort.Slice(sl.ContactInfos, func (i, j int) bool {
		c1 := sl.ContactInfos[i]
		c2 := sl.ContactInfos[j]

		d1 := sl.TargetID.Xor(c1.Node.NodeID)
		d2 := sl.TargetID.Xor(c2.Node.NodeID)

		return d1.Less(d2)
	})
	index := 0
	// replace inactive contacts in ShortList with closer contact
	for _, con := range contacts {
		for i, conInfo := range sl.ContactInfos {
			if conInfo.Status == false {
				d1 := sl.TargetID.Xor(con.NodeID)
				d2 := sl.TargetID.Xor(conInfo.Node.NodeID)

				// replace the current inactive conInfo with the closer input contact
				if d1.Less(d2) {
					isClosestChanged = (isClosestChanged || sl.ReplaceInactiveContact(i, con))
					index += 1
					break
				}
			}
		}
	}

	// still has spot in shortlist
	for len(sl.ContactInfos) < sl.Capacity && index < len(contacts) {
		// add rest input contacts，if any
		isClosestChanged = (isClosestChanged || sl.AddContact(contacts[index]))
		index += 1
	}

	log.Println("AddContacts finished: ")
	<- sl.sem
	return isClosestChanged
}

// Remove inactive contact from ShortList
// Return error if contact is not found in ShortList or trying to remove active contact
func (sl *ShortList) RemoveContact(contact Contact) error {
	sl.sem <- 1
	size := len(sl.ContactInfos)

	for i := 0; i < size; i++ {
		if contact.NodeID.Equals(sl.ContactInfos[i].Node.NodeID) {
			if sl.ContactInfos[i].Status == true {
				errors.New("Trying to remove active ID: " + contact.NodeID.AsString())
			}
			sl.ContactInfos = append(sl.ContactInfos[:i], sl.ContactInfos[i+1:]...)
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
	count := 0
	for _, coninfo := range sl.ContactInfos {
		if coninfo.Status == false {
			contacts = append(contacts, coninfo.Node)
			count += 1
		}
	}

	<-sl.sem
	return contacts
}

func (sl *ShortList) MarkContactAsActive(c Contact) error {
	sl.sem <- 1

	for i := 0; i < len(sl.ContactInfos); i++ {
		if sl.ContactInfos[i].Node.NodeID.Equals(c.NodeID) {
			if sl.ContactInfos[i].Status == true {
				log.Println("Trying to mark active contact as active; might be a parallel problem ...")
			} else {
				sl.ContactInfos[i].Status = true
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

	count := 0
	for _, coninfo := range sl.ContactInfos {
		if coninfo.Status == false {
			count += 1
		}
	}

	<-sl.sem
	return count
}
