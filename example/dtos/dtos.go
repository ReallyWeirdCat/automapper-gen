package dtos

type Interest int

const (
	Gaming Interest = iota
	Programming
	Cooking
	Painting
	Movies
)

type Role int

const (
	Client Role = iota
	Cashier
	Restocker
	Delivery
	Security
	Tech
)

//automapper:from=db.UserDB
type UserDTO struct {
	ID        int64
	Username  string
	Role      Role       `automapper:"converter=RoleEnum"`
	About     string     // pointer mismatch!
	Interests []Interest `automapper:"converter=InterestEnums"`
	Birthday  *string    `automapper:"converter=TimeToString"`
	CreatedAt string     `automapper:"converter=TimeToString"`
}
