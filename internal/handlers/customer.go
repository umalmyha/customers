package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/customer"
	"github.com/umalmyha/customers/internal/service"
	"net/http"
)

type NewCustomer struct {
	FirstName  string              `json:"firstName"`
	LastName   string              `json:"lastName"`
	MiddleName *string             `json:"middleName"`
	Email      string              `json:"email"`
	Importance customer.Importance `json:"importance"`
	Inactive   bool                `json:"inactive"`
}

type UpdateCustomer struct {
	Id         string              `param:"id"`
	FirstName  string              `json:"firstName"`
	LastName   string              `json:"lastName"`
	MiddleName *string             `json:"middleName"`
	Email      string              `json:"email"`
	Importance customer.Importance `json:"importance"`
	Inactive   bool                `json:"inactive"`
}

type CustomerHandler struct {
	custSrv service.CustomerService
}

func NewCustomerHandler(custSrv service.CustomerService) *CustomerHandler {
	return &CustomerHandler{custSrv: custSrv}
}

func (h *CustomerHandler) Get(c echo.Context) error {
	cust, err := h.custSrv.FindById(c.Request().Context(), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if cust.Id == "" {
		return c.JSON(http.StatusOK, nil)
	}
	return c.JSON(http.StatusOK, &cust)
}

func (h *CustomerHandler) GetAll(c echo.Context) error {
	customers, err := h.custSrv.FindAll(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, customers)
}

func (h *CustomerHandler) Post(c echo.Context) error {
	var nc NewCustomer
	if err := c.Bind(&nc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	cust, err := h.custSrv.Create(c.Request().Context(), customer.Customer{
		FirstName:  nc.FirstName,
		LastName:   nc.LastName,
		MiddleName: nc.MiddleName,
		Email:      nc.Email,
		Importance: nc.Importance,
		Inactive:   nc.Inactive,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusCreated, &cust)
}

func (h *CustomerHandler) Put(c echo.Context) error {
	var uc UpdateCustomer
	if err := c.Bind(&uc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	cust, err := h.custSrv.Upsert(c.Request().Context(), customer.Customer(uc))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, &cust)
}

func (h *CustomerHandler) DeleteById(c echo.Context) error {
	id := c.Param("id")
	if err := h.custSrv.DeleteById(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
