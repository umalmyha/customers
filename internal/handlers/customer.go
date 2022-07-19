package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/umalmyha/customers/internal/service"
	"net/http"
)

type newCustomer struct {
	FirstName  string              `json:"firstName"`
	LastName   string              `json:"lastName"`
	MiddleName *string             `json:"middleName"`
	Email      string              `json:"email"`
	Importance customer.Importance `json:"importance"`
	Inactive   bool                `json:"inactive"`
}

type updateCustomer struct {
	Id         string              `param:"id"`
	FirstName  string              `json:"firstName"`
	LastName   string              `json:"lastName"`
	MiddleName *string             `json:"middleName"`
	Email      string              `json:"email"`
	Importance customer.Importance `json:"importance"`
	Inactive   bool                `json:"inactive"`
}

type CustomerHandler struct {
	customerSvc service.CustomerService
}

func NewCustomerHandler(customerSvc service.CustomerService) *CustomerHandler {
	return &CustomerHandler{customerSvc: customerSvc}
}

func (h *CustomerHandler) Get(c echo.Context) error {
	customer, err := h.customerSvc.FindById(c.Request().Context(), c.Param("id"))
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

	upsertCustomer := customer.Customer(uc)
	customer, err := h.customerSvc.Upsert(c.Request().Context(), &upsertCustomer)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, &customer)
}

func (h *CustomerHandler) DeleteById(c echo.Context) error {
	if err := h.customerSvc.DeleteById(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
