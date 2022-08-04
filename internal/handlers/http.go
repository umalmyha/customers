package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/service"
)

const mimeBytesNumber = 512

type session struct {
	Token        string `json:"accessToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	RefreshToken string `json:"refreshToken"`
}

type signup struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=4,max=24"`
}

type logout struct {
	RefreshToken string `json:"refreshToken" validate:"required,uuid"`
}

type newUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type login struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required"`
	Fingerprint string `json:"fingerprint" validate:"required"`
}

type refresh struct {
	Fingerprint  string `json:"fingerprint" validate:"required"`
	RefreshToken string `json:"refreshToken" validate:"required,uuid"`
}

// AuthHTTPHandler is http handler for auth endpoint
type AuthHTTPHandler struct {
	authSvc service.AuthService
}

// NewAuthHTTPHandler builds new AuthHTTPHandler
func NewAuthHTTPHandler(authSvc service.AuthService) *AuthHTTPHandler {
	return &AuthHTTPHandler{
		authSvc: authSvc,
	}
}

// Signup signups new user
// @Summary     Signup new account
// @Description Register new account based on provided credentials
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       signup body	    signup true "New user data"
// @Success     200    {object} newUser
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/auth/signup [post]
func (h *AuthHTTPHandler) Signup(c echo.Context) error {
	var su signup
	if err := c.Bind(&su); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&su); err != nil {
		return err
	}

	nu, err := h.authSvc.Signup(c.Request().Context(), su.Email, su.Password)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &newUser{
		ID:    nu.ID,
		Email: nu.Email,
	})
}

// Login logins user
// @Summary     Login user
// @Description Verifies provided credentials, sign auth and refresh token
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       login  body	    login true "User credentials"
// @Success     200    {object} session
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/auth/login [post]
func (h *AuthHTTPHandler) Login(c echo.Context) error {
	var lgn login
	if err := c.Bind(&lgn); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&lgn); err != nil {
		return err
	}

	jwt, rfrToken, err := h.authSvc.Login(c.Request().Context(), lgn.Email, lgn.Password, lgn.Fingerprint, time.Now().UTC())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &session{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.ID,
	})
}

// Logout logouts user
// @Summary     Logout user
// @Description Remove any user-related session data
// @Tags        auth
// @Accept      json
// @Param       logout body	    logout true "Refresh token id"
// @Success     200    "Successful status code"
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/auth/logout [post]
func (h *AuthHTTPHandler) Logout(c echo.Context) error {
	var lgt logout
	if err := c.Bind(&lgt); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&lgt); err != nil {
		return err
	}

	if err := h.authSvc.Logout(c.Request().Context(), lgt.RefreshToken); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// Refresh refreshes user session
// @Summary     Refresh auth
// @Description Sign new auth and refresh token
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       refresh body	 refresh true "Fingerprint and refresh token id"
// @Success     200     {object} session
// @Failure     400     {object} echo.HTTPError
// @Failure     500     {object} echo.HTTPError
// @Router      /api/auth/refresh [post]
func (h *AuthHTTPHandler) Refresh(c echo.Context) error {
	var r refresh
	if err := c.Bind(&r); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&r); err != nil {
		return err
	}

	jwt, rfrToken, err := h.authSvc.Refresh(c.Request().Context(), r.RefreshToken, r.Fingerprint, time.Now().UTC())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &session{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.ID,
	})
}

type identifier struct {
	ID string `json:"id" validate:"required,uuid"`
}

type newCustomer struct {
	FirstName  string           `json:"firstName" validate:"required"`
	LastName   string           `json:"lastName" validate:"required"`
	MiddleName *string          `json:"middleName"`
	Email      string           `json:"email" validate:"required,email"`
	Importance model.Importance `json:"importance" validate:"required,oneof=1 2 3 4"`
	Inactive   bool             `json:"inactive"`
}

type updateCustomer struct {
	ID string `param:"id" validate:"required,uuid"`
	newCustomer
}

// CustomerHTTPHandler is http handler for customer endpoint
type CustomerHTTPHandler struct {
	customerSvc service.CustomerService
}

// NewCustomerHTTPHandler builds new CustomerHTTPHandler
func NewCustomerHTTPHandler(customerSvc service.CustomerService) *CustomerHTTPHandler {
	return &CustomerHTTPHandler{customerSvc: customerSvc}
}

// Get gets user
// @Summary     Get single customer by id
// @Description Returns single customer with provided id
// @Tags        customers
// @Security	ApiKeyAuth
// @Produce     json
// @Param       id     query 	string true "Customer guid" Format(uuid)
// @Success     200    {object} customer.Customer
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/v1/customers/{id} [get]
// @Router      /api/v2/customers/{id} [get]
func (h *CustomerHTTPHandler) Get(c echo.Context) error {
	id := c.Param("id")
	if err := c.Validate(&identifier{ID: id}); err != nil {
		return err
	}

	customer, err := h.customerSvc.FindByID(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, customer)
}

// GetAll gets all users
// @Summary     Get all customers
// @Description Returns all customers
// @Tags        customers
// @Security	ApiKeyAuth
// @Produce     json
// @Success     200    {array}  customer.Customer
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/v1/customers [get]
// @Router      /api/v2/customers [get]
func (h *CustomerHTTPHandler) GetAll(c echo.Context) error {
	customers, err := h.customerSvc.FindAll(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, customers)
}

// Post creates new customer
// @Summary     New Customer
// @Description Creates new customer
// @Tags        customers
// @Security	ApiKeyAuth
// @Accept		json
// @Produce     json
// @Param 		newCustomer body	 newCustomer true "Data for new customer"
// @Success     200    		{object} customer.Customer
// @Failure     400    		{object} echo.HTTPError
// @Failure     500    		{object} echo.HTTPError
// @Router      /api/v1/customers [post]
// @Router      /api/v2/customers [post]
func (h *CustomerHTTPHandler) Post(c echo.Context) error {
	var nc newCustomer
	if err := c.Bind(&nc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&nc); err != nil {
		return err
	}

	customer, err := h.customerSvc.Create(c.Request().Context(), &model.Customer{
		FirstName:  nc.FirstName,
		LastName:   nc.LastName,
		MiddleName: nc.MiddleName,
		Email:      nc.Email,
		Importance: nc.Importance,
		Inactive:   nc.Inactive,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, customer)
}

// Put updates/creates customer
// @Summary     Update/Create Customer
// @Description Updates customer or creates new if not exist
// @Tags        customers
// @Security	ApiKeyAuth
// @Accept		json
// @Produce     json
// @Param       id     		   query 	string 		   true "Customer guid" Format(uuid)
// @Param 		updateCustomer body	    updateCustomer true "Customer data"
// @Success     200    		   {object} customer.Customer
// @Failure     400    		   {object} echo.HTTPError
// @Failure     500    		   {object} echo.HTTPError
// @Router      /api/v1/customers/{id} [put]
// @Router      /api/v2/customers/{id} [put]
func (h *CustomerHTTPHandler) Put(c echo.Context) error {
	var uc updateCustomer
	if err := c.Bind(&uc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&uc); err != nil {
		return err
	}

	customer, err := h.customerSvc.Upsert(c.Request().Context(), &model.Customer{
		ID:         uc.ID,
		FirstName:  uc.FirstName,
		LastName:   uc.LastName,
		MiddleName: uc.MiddleName,
		Email:      uc.Email,
		Importance: uc.Importance,
		Inactive:   uc.Inactive,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &customer)
}

// DeleteByID deletes customer
// @Summary     Delete customer by id
// @Description Deletes customer with provided id
// @Tags        customers
// @Security	ApiKeyAuth
// @Produce     json
// @Param       id     query 	string true "Customer guid" Format(uuid)
// @Success     204    "Successful status code"
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/v1/customers/{id} [delete]
// @Router      /api/v2/customers/{id} [delete]
func (h *CustomerHTTPHandler) DeleteByID(c echo.Context) error {
	id := c.Param("id")
	if err := c.Validate(&identifier{ID: id}); err != nil {
		return err
	}

	if err := h.customerSvc.DeleteByID(c.Request().Context(), id); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// ImageHTTPHandler is http handler for image endpoint
type ImageHTTPHandler struct {
	validImgMimeTypes map[string]struct{}
}

// NewImageHTTPHandler builds new ImageHTTPHandler
func NewImageHTTPHandler() *ImageHTTPHandler {
	return &ImageHTTPHandler{
		validImgMimeTypes: map[string]struct{}{
			"image/gif":                {},
			"image/jpeg":               {},
			"image/pjpeg":              {},
			"image/png":                {},
			"image/svg+xml":            {},
			"image/tiff":               {},
			"image/vnd.microsoft.icon": {},
			"image/vnd.wap.wbmp":       {},
			"image/webp":               {},
		},
	}
}

// Upload uploads image
// @Summary     Upload image
// @Description Uploads image to the server
// @Tags        images
// @Accept		mpfd
// @Param 		image formData file true "Image"
// @Success     200   "Successful status code"
// @Failure     400   {object} echo.HTTPError
// @Failure     500   {object} echo.HTTPError
// @Router      /images/upload [post]
func (h *ImageHTTPHandler) Upload(c echo.Context) (err error) {
	fileHdr, err := c.FormFile("image")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	file, err := fileHdr.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to load file content - %v", err))
	}

	mimeBuff := make([]byte, mimeBytesNumber)
	_, err = file.Read(mimeBuff)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	mimeType := http.DetectContentType(mimeBuff)
	if !h.isMimeTypeAllowed(mimeType) {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("MIME type %s is not allowed", mimeType))
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	path := fmt.Sprintf("./images/%s", fileHdr.Filename)
	dst, err := os.Create(filepath.Clean(path))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if _, err := io.Copy(dst, file); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := file.Close(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := dst.Close(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusOK)
}

// Download downloads image
// @Summary     Download image
// @Description Downloads image from the server
// @Tags        images
// @Produce		image/gif
// @Produce		image/jpeg
// @Produce		image/pjpeg
// @Produce		image/png
// @Produce		image/svg+xml
// @Produce		image/tiff
// @Produce		image/vnd.microsoft.icon
// @Produce		image/vnd.wap.wbmp
// @Produce		image/webp
// @Param 		name  query    string true "Image name"
// @Success     200   {string} file
// @Failure     400   {object} echo.HTTPError
// @Failure     500   {object} echo.HTTPError
// @Router      /images/{name}/download [get]
func (h *ImageHTTPHandler) Download(c echo.Context) error {
	name := c.Param("name")
	path := fmt.Sprintf("./images/%s", name)
	return c.Attachment(path, name)
}

func (h *ImageHTTPHandler) isMimeTypeAllowed(mime string) bool {
	if _, ok := h.validImgMimeTypes[mime]; ok {
		return true
	}
	return false
}
