package validation

import (
	"bytes"
	"encoding/json"
	"errors"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"net/http"
)

type violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type PayloadError struct {
	violations []violation
}

func (e *PayloadError) Error() string {
	buff := bytes.NewBufferString("")

	for _, err := range e.violations {
		buff.WriteString(err.Message)
		buff.WriteString("\n")
	}

	return buff.String()
}

func (e *PayloadError) Violation(v violation) {
	e.violations = append(e.violations, v)
}

func (e *PayloadError) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Errors []violation `json:"errors"`
	}{
		Errors: e.violations,
	})
}

type EchoValidator struct {
	validator  *validator.Validate
	translator ut.Translator
}

func Echo(validator *validator.Validate, translator ut.Translator) *EchoValidator {
	return &EchoValidator{
		validator:  validator,
		translator: translator,
	}
}

func (v *EchoValidator) Validate(i any) error {
	err := v.validator.Struct(i)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		return v.payloadError(ve)
	}

	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}

func (v *EchoValidator) payloadError(ve validator.ValidationErrors) error {
	pldErr := &PayloadError{violations: make([]violation, 0)}
	for _, e := range ve {
		pldErr.Violation(violation{
			Field:   e.Field(),
			Message: e.Translate(v.translator),
		})
	}
	return pldErr
}
