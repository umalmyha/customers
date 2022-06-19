package handlers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/customer"
	"github.com/umalmyha/customers/internal/errors"
	"github.com/umalmyha/customers/internal/service"
	"net/http"
)

var DeserializeRequestPayloadErr = func(reason string) error {
	return errors.NewBusinessErr("payload", fmt.Sprintf("failed to deserialize request pyaload - %s", reason))
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
		return err
	}
	return c.JSON(http.StatusOK, &cust)
}

func (h *CustomerHandler) GetAll(c echo.Context) error {
	customers, err := h.custSrv.FindAll(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, customers)
}

func (h *CustomerHandler) Post(c echo.Context) error {
	var newCust customer.NewCustomer
	if err := c.Bind(&newCust); err != nil {
		return DeserializeRequestPayloadErr(err.Error())
	}

	cust, err := h.custSrv.Create(c.Request().Context(), newCust)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, &cust)
}

func (h *CustomerHandler) Patch(c echo.Context) error {
	var patchCust customer.PatchCustomer
	if err := c.Bind(&patchCust); err != nil {
		return DeserializeRequestPayloadErr(err.Error())
	}

	cust, err := h.custSrv.Merge(c.Request().Context(), patchCust)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, &cust)
}

func (h *CustomerHandler) Put(c echo.Context) error {
	var updCust customer.UpdateCustomer
	if err := c.Bind(&updCust); err != nil {
		return DeserializeRequestPayloadErr(err.Error())
	}

	cust, err := h.custSrv.Upsert(c.Request().Context(), updCust)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, &cust)
}

func (h *CustomerHandler) DeleteById(c echo.Context) error {
	id := c.Param("id")
	if err := h.custSrv.DeleteById(c.Request().Context(), id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
