package user

import (
	"time"

	"github.com/cymonevo/go_template/pkg/auth"
)

// CreateUserInput is the validated payload for creating a user.
type CreateUserInput struct {
	Email    string `json:"email" validate:"required,email"`
	Name     string `json:"name" validate:"required,min=2,max=120"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// UpdateUserInput is the validated payload for updating a user.
type UpdateUserInput struct {
	Name string `json:"name" validate:"required,min=2,max=120"`
}

// LoginInput is the validated payload for authentication.
type LoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RefreshInput is the validated payload for refreshing tokens.
type RefreshInput struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ListUsersInput carries pagination and filtering options.
type ListUsersInput struct {
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	Search  string `json:"search"`
}

// Response is the public representation of a user (never exposes the password).
type Response struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      auth.Role `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToResponse maps a domain entity to its public representation.
func ToResponse(u *User) Response {
	return Response{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// ToResponses maps a slice of entities.
func ToResponses(users []User) []Response {
	out := make([]Response, 0, len(users))
	for i := range users {
		out = append(out, ToResponse(&users[i]))
	}
	return out
}

// PageMeta is pagination metadata returned alongside list responses.
type PageMeta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}
