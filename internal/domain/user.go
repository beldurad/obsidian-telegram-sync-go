package domain

const (
	StatusRegistered = "registered"
	StatusAnonimous  = "anonimous"
)

const UserContextKey = "user"

type User struct {
	ID             int64
	TelegramID     int64
	RegisterStatus string
}

func AnonimousUser(telegramID int64) *User {
	return &User{
		TelegramID:     telegramID,
		RegisterStatus: StatusAnonimous,
	}
}

func (u *User) IsAnonimous() bool {
	return u == nil || u.RegisterStatus == StatusAnonimous
}

type UserService interface {
	// FindByTelegramID() returns a [User] by it's telegram chat ID
	FindByTelegramID(telegramID int64) (*User, error)
}
