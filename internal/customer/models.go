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

func (cust Customer) IsInitial() bool {
	return cust == (Customer{})
}

func (cust Customer) MergePatch(patch PatchCustomer) Customer {
	if patch.FirstName != nil {
		cust.FirstName = *patch.FirstName
	}

	if patch.LastName != nil {
		cust.LastName = *patch.LastName
	}

	if patch.MiddleName != nil {
		s := *patch.MiddleName
		cust.MiddleName = &s
	}

	if patch.Email != nil {
		cust.Email = *patch.Email
	}

	if patch.Importance != nil {
		cust.Importance = *patch.Importance
	}

	if patch.Inactive != nil {
		cust.Inactive = *patch.Inactive
	}
	return cust
}

type NewCustomer struct {
	FirstName  string     `json:"firstName"`
	LastName   string     `json:"lastName"`
	MiddleName *string    `json:"middleName"`
	Email      string     `json:"email"`
	Importance Importance `json:"importance"`
	Inactive   bool       `json:"inactive"`
}

type UpdateCustomer struct {
	Id         string     `param:"id"`
	FirstName  string     `json:"firstName"`
	LastName   string     `json:"lastName"`
	MiddleName *string    `json:"middleName"`
	Email      string     `json:"email"`
	Importance Importance `json:"importance"`
	Inactive   bool       `json:"inactive"`
}

type PatchCustomer struct {
	Id         string      `param:"id"`
	FirstName  *string     `json:"firstName"`
	LastName   *string     `json:"lastName"`
	MiddleName *string     `json:"middleName"`
	Email      *string     `json:"email"`
	Importance *Importance `json:"importance"`
	Inactive   *bool       `json:"inactive"`
}
