package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
)

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UpdateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (v ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", v.Field, v.Message)
}

type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

func DecodeAndValidate[T any](r *http.Request, validate func(*T) ValidationErrors) (*T, error) {
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if errs := validate(&req); len(errs) > 0 {
		return nil, errs
	}

	return &req, nil
}

func ValidateCreateUser(req *CreateUserRequest) ValidationErrors {
	var errs ValidationErrors

	if strings.TrimSpace(req.Name) == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "Name is required"})
	}

	if strings.TrimSpace(req.Email) == "" {
		errs = append(errs, ValidationError{Field: "email", Message: "Valid email is required"})
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		errs = append(errs, ValidationError{Field: "email", Message: "Valid email is required"})
	}

	return errs
}

func ValidateUpdateUser(req *UpdateUserRequest) ValidationErrors {
	var errs ValidationErrors

	if strings.TrimSpace(req.Name) == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "Name is required"})
	}

	if strings.TrimSpace(req.Email) == "" {
		errs = append(errs, ValidationError{Field: "email", Message: "Valid email is required"})
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		errs = append(errs, ValidationError{Field: "email", Message: "Valid email is required"})
	}

	return errs
}
