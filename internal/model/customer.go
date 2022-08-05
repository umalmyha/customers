package model

// Importance specifies how important customer is
type Importance int

const (
	// ImportanceLow means low customer importance
	ImportanceLow Importance = iota
	// ImportanceMedium means medium customer importance
	ImportanceMedium
	// ImportanceHigh means high customer importance
	ImportanceHigh
	// ImportanceCritical means critical customer importance
	ImportanceCritical
)

// Customer is customer model entity
type Customer struct {
	ID         string     `json:"id" bson:"_id,omitempty"`
	FirstName  string     `json:"firstName" bson:"firstName"`
	LastName   string     `json:"lastName" bson:"lastName"`
	MiddleName *string    `json:"middleName" bson:"middleName"`
	Email      string     `json:"email" bson:"email"`
	Importance Importance `json:"importance" bson:"importance"`
	Inactive   bool       `json:"inactive" bson:"inactive"`
}
