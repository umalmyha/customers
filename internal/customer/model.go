package customer

type Importance int

const (
	ImportanceLow Importance = iota + 1
	ImportanceMedium
	ImportanceHigh
	ImportanceCritical
)

type Customer struct {
	Id         string     `json:"id"`
	FirstName  string     `json:"firstName"`
	LastName   string     `json:"lastName"`
	MiddleName *string    `json:"middleName"`
	Email      string     `json:"email"`
	Importance Importance `json:"importance"`
	Inactive   bool       `json:"inactive"`
}
