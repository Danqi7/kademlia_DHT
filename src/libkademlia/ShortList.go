package libkademlia

import (
	"errors"
	"log"
)

type ShortList struct {
	Contacts    []Contact
	Stata       []bool
	Closest     *Contact
	NumInactive int
	sem         chan int
	capacity    int
}

func (sl *ShortList) Init(k int) {
	sl.Contacts = make([]Contact, 0, k)
	sl.Stata = make([]bool, 0, k)
	sl.sem = make(chan int, 1)
	sl.capacity = k
	sl.Closest = nil
	sl.NumInactive = 0
}

func (sl *ShortList) UpdateClosest(position int) bool {
	if sl.Closest == nil {
		/////// DEBUG ->
		log.Printf("UpdateClosest: default case; position: %d; ID: %s\n", position, sl.Contacts[position].NodeID.AsString())
		/////// <- DEBUG
		sl.Closest = &sl.Contacts[position]
		return true
	}
	if sl.Contacts[position].NodeID.Compare(sl.Closest.NodeID) <= 0 {
		/////// DEBUG ->
		log.Printf("UpdateClosest: real update: position: %d; ID: %s\n", position, sl.Contacts[position].NodeID.AsString())
		/////// <- DEBUG
		sl.Closest = &sl.Contacts[position]
		return true
	}
	/////// DEBUG ->
	log.Printf("UpdateClosest: failed to update: position: %d; ID: %s\n", position, sl.Contacts[position].NodeID.AsString())
	/////// <- DEBUG
	return false
}

func (sl *ShortList) Add(c Contact) bool {
	var isClosestChanged bool = false
	sl.sem <- 1
	contactLength := len(sl.Contacts)
	modifyPosition := -1
	// check for replacement and existing
	for i := 0; i < contactLength; i++ {
		if modifyPosition == -1 && c.NodeID.Compare(sl.Contacts[i].NodeID) < 0 {
			modifyPosition = i
		} else if c.NodeID.Equals(sl.Contacts[i].NodeID) {
			<-sl.sem
			return false
		}
	}
	// at capacity
	if contactLength == sl.capacity {
		// cannot replace anything
		if modifyPosition == -1 {
			/////// DEBUG ->
			log.Printf("Add: at capacity and cannot replace anything. ID of the attempted: %s\n", c.NodeID.AsString())
			/////// <- DEBUG
			<-sl.sem
			return false
		} else {
			// can replace something
			/////// DEBUG ->
			log.Printf("Add: updating ID: %s with ID: %s\n", sl.Contacts[modifyPosition].NodeID.AsString, c.NodeID.AsString())
			/////// <- DEBUG
			// replace the far one and mark it as active
			sl.Contacts[modifyPosition] = c
			if sl.Stata[modifyPosition] == true {
				sl.NumInactive++
			}
			sl.Stata[modifyPosition] = false
			isClosestChanged = sl.UpdateClosest(modifyPosition)
			<-sl.sem
			return isClosestChanged
		}
	}

	// add at the tail
	/////// DEBUG ->
	log.Printf("Add: adding ID: %s\n", c.NodeID.AsString())
	/////// <- DEBUG
	sl.Contacts = append(sl.Contacts, c)
	sl.Stata = append(sl.Stata, false)
	sl.NumInactive++
	isClosestChanged = sl.UpdateClosest(len(sl.Contacts) - 1)
	<-sl.sem
	log.Printf("Add: added; sl length: %d\n", len(sl.Contacts))
	return isClosestChanged
}

func (sl *ShortList) AddAlpha(cs []Contact, a int) bool {
	size := len(cs)
	if len(cs) == 0 {
		return false
	}
	if size > a {
		size = a
	}
	isClosestChanged := false
	for i := 0; i < size; i++ {
		/////// DEBUG ->
		log.Printf("AddAlpha: adding %d of %d\n", i+1, size)
		/////// <- DEBUG
		// add the contact to the short list
		isClosestChanged = (sl.Add(cs[i]) || isClosestChanged)
	}
	return isClosestChanged
}

func (sl *ShortList) RemoveContact(c Contact) error {
	sl.sem <- 1
	i := 0
	size := len(sl.Contacts)
	for ; i < size; i++ {
		if c.NodeID.Equals(sl.Contacts[i].NodeID) {
			if sl.Stata[i] == false {
				sl.NumInactive--
			}
			sl.Contacts = append(sl.Contacts[:i], sl.Contacts[i+1:]...)
			sl.Stata = append(sl.Stata[:i], sl.Stata[i+1:]...)
			<-sl.sem
			return nil
		}
	}
	<-sl.sem
	return errors.New("ID " + c.NodeID.AsString() + " not found among ShortList contacts (RemoveContact)")
}

func (sl *ShortList) GetNextAlphaInactive(a int) []Contact {
	sl.sem <- 1
	/////// DEBUG ->
	log.Printf("GetNextAlphaInactive: starting... alpha = %d, contacts size = %d\n", a, len(sl.Contacts))
	/////// <- DEBUG
	if len(sl.Contacts) == 0 {
		<-sl.sem
		/////// DEBUG ->
		log.Printf("GetNextAlphaInactive: sl.contacts is empty... returning\n")
		/////// <- DEBUG
		return nil
	}
	cs := make([]Contact, 0)
	size := len(sl.Contacts)
	if size > a {
		size = a
	}
	for i := 0; i < size; i++ {
		/////// DEBUG ->
		log.Printf("GetNextAlphaInactive: checking position %d: %t - ID: %s\n", i, sl.Stata[i], sl.Contacts[i].NodeID.AsString())
		/////// <- DEBUG
		if sl.Stata[i] == false {
			/////// DEBUG ->
			log.Printf("GetNextAlphaInactive: found @ position %d: %t\n", i, sl.Stata[i])
			/////// <- DEBUG
			cs = append(cs, sl.Contacts[i])
			if len(cs) == a {
				/////// DEBUG ->
				log.Printf("GetNextAlphaInactive: cs has enough length: %s\n", len(cs))
				/////// <- DEBUG
				<-sl.sem
				return cs
			}
		}
	}
	<-sl.sem
	return cs
}

func (sl *ShortList) MarkContactAsActive(c Contact) error {
	sl.sem <- 1
	i := 0
	size := len(sl.Contacts)
	for ; i < size; i++ {
		if sl.Contacts[i].NodeID.Equals(c.NodeID) {
			sl.Stata[i] = true
			sl.NumInactive--
			<-sl.sem
			return nil
		}
	}
	<-sl.sem
	return errors.New("ID " + c.NodeID.AsString() + " not found among ShortList contacts (MarkContactAsActive)")
}
