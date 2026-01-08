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
	ID                  int64
	Username            string
	Role                Role `automapper:"converter=RoleEnum"`
	About               string
	Pets                []PetDTO       `automapper:"dto=PetDTO"`
	FeaturedAchievement AchievementDTO `automapper:"dto=AchievementDTO"`
	Interests           []Interest     `automapper:"converter=InterestEnums"`
	Birthday            *string        `automapper:"converter=TimeToString"`
	CreatedAt           string         `automapper:"converter=TimeToString"`
}

//automapper:from=db.PetDB
type PetDTO struct {
	ID        int64
	Name      string
	Interests []Interest `automapper:"converter=InterestEnums"`
	Birthday  *string    `automapper:"converter=TimeToString"`
	CreatedAt string     `automapper:"converter=TimeToString"`
}

//automapper:from=db.AchievementDB
type AchievementDTO struct {
	ID          int64
	Title       string
	Description string `automapper:"converter=ToLower"`
}
