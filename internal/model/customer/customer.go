package customer

type Importance int

const (
	ImportanceLow Importance = iota
	ImportanceMedium
	ImportanceHigh
	ImportanceCritical
)

type Customer struct {
	Id         string     `json:"id" bson:"_id,omitempty"`
	FirstName  string     `json:"firstName" bson:"firstName"`
	LastName   string     `json:"lastName" bson:"lastName"`
	MiddleName *string    `json:"middleName" bson:"middleName"`
	Email      string     `json:"email" bson:"email"`
	Importance Importance `json:"importance" bson:"importance"`
	Inactive   bool       `json:"inactive" bson:"inactive"`
}
