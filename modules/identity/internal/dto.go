package identity

type CreateUserDto struct {
	FirstName   string `json:"firstName"   validate:"required,max=50"`
	LastName    string `json:"lastName"    validate:"omitempty,max=50"`
	Email       string `json:"email"       validate:"required,email,max=255"`
	Password    string `json:"password"    validate:"required,min=8,max=72,password"`
	PhoneNumber string `json:"phoneNumber" validate:"required,e164"`
}

type GetUserDto struct {
	ID          string `json:"id"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
}

type UserLoginReqDto struct {
	Email    string `json:"email"    validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72,password"`
}

type UserLoginResDto struct {
	Token string `json:"token"`
	GetUserDto
}
