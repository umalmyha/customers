package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/umalmyha/customers/internal/service"
	"net/http"
)

type identifier struct {
	Id string `json:"id" validate:"required,uuid"`
}

type newCustomer struct {
	FirstName  string              `json:"firstName" validate:"required"`
	LastName   string              `json:"lastName" validate:"required"`
	MiddleName *string             `json:"middleName"`
	Email      string              `json:"email" validate:"required,email"`
	Importance customer.Importance `json:"importance" validate:"required,oneof=1 2 3 4"`
	Inactive   bool                `json:"inactive"`
}

type updateCustomer struct {
	Id string `param:"id" validate:"required,uuid"`
	newCustomer
}

type CustomerHandler struct {
	customerSvc service.CustomerService
}

func NewCustomerHandler(customerSvc service.CustomerService) *CustomerHandler {
	return &CustomerHandler{customerSvc: customerSvc}
}

func (h *CustomerHandler) Get(c echo.Context) error {
	id := c.Param("id")
	if err := c.Validate(&identifier{Id: id}); err != nil {
		return err
	}

	customer, err := h.customerSvc.FindById(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, customer)
}

func (h *CustomerHandler) GetAll(c echo.Context) error {
	customers, err := h.customerSvc.FindAll(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, customers)
}

func (h *CustomerHandler) Post(c echo.Context) error {
	var nc newCustomer
	if err := c.Bind(&nc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&nc); err != nil {
		return err
	}

	customer, err := h.customerSvc.Create(c.Request().Context(), &customer.Customer{
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

func (h *CustomerHandler) Put(c echo.Context) error {
	var uc updateCustomer
	if err := c.Bind(&uc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&uc); err != nil {
		return err
	}

	customer, err := h.customerSvc.Upsert(c.Request().Context(), &customer.Customer{
		Id:         uc.Id,
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

func (h *CustomerHandler) DeleteById(c echo.Context) error {
	id := c.Param("id")
	if err := c.Validate(&identifier{Id: id}); err != nil {
		return err
	}

	if err := h.customerSvc.DeleteById(c.Request().Context(), id); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
