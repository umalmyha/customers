package handlers

import (
	"context"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/umalmyha/customers/internal/proto"
	"github.com/umalmyha/customers/internal/service"
	"google.golang.org/protobuf/types/known/emptypb"
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

type CustomerHttpHandler struct {
	customerSvc service.CustomerService
}

func NewCustomerHttpHandler(customerSvc service.CustomerService) *CustomerHttpHandler {
	return &CustomerHttpHandler{customerSvc: customerSvc}
}

// Get godoc
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
func (h *CustomerHttpHandler) Get(c echo.Context) error {
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

// GetAll godoc
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
func (h *CustomerHttpHandler) GetAll(c echo.Context) error {
	customers, err := h.customerSvc.FindAll(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, customers)
}

// Post godoc
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
func (h *CustomerHttpHandler) Post(c echo.Context) error {
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

// Put godoc
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
func (h *CustomerHttpHandler) Put(c echo.Context) error {
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

// DeleteById godoc
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
func (h *CustomerHttpHandler) DeleteById(c echo.Context) error {
	id := c.Param("id")
	if err := c.Validate(&identifier{Id: id}); err != nil {
		return err
	}

	if err := h.customerSvc.DeleteById(c.Request().Context(), id); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

type customerGrpcHandler struct {
	proto.UnimplementedCustomerServiceServer
	customerSvc service.CustomerService
}

func NewCustomerGrpcHandler(customerSvc service.CustomerService) *customerGrpcHandler {
	return &customerGrpcHandler{
		UnimplementedCustomerServiceServer: proto.UnimplementedCustomerServiceServer{},
		customerSvc:                        customerSvc,
	}
}

func (h *customerGrpcHandler) GetById(ctx context.Context, req *proto.GetCustomerByIdRequest) (*proto.CustomerResponse, error) {
	c, err := h.customerSvc.FindById(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return h.customerResponse(c), nil
}

func (h *customerGrpcHandler) GetAll(ctx context.Context, _ *emptypb.Empty) (*proto.CustomerListResponse, error) {
	customers, err := h.customerSvc.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	res := make([]*proto.CustomerResponse, 0)
	for _, c := range customers {
		res = append(res, h.customerResponse(c))
	}

	return &proto.CustomerListResponse{Customers: res}, nil
}

func (h *customerGrpcHandler) Create(ctx context.Context, req *proto.NewCustomerRequest) (*proto.CustomerResponse, error) {
	c, err := h.customerSvc.Create(ctx, &customer.Customer{
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		MiddleName: req.MiddleName,
		Email:      req.Email,
		Importance: customer.Importance(req.Importance),
		Inactive:   req.Inactive,
	})
	if err != nil {
		return nil, err
	}

	return h.customerResponse(c), nil
}

func (h *customerGrpcHandler) Upsert(ctx context.Context, req *proto.UpdateCustomerRequest) (*proto.CustomerResponse, error) {
	c, err := h.customerSvc.Upsert(ctx, &customer.Customer{
		Id:         req.Id,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		MiddleName: req.MiddleName,
		Email:      req.Email,
		Importance: customer.Importance(req.Importance),
		Inactive:   req.Inactive,
	})
	if err != nil {
		return nil, err
	}

	return h.customerResponse(c), nil
}

func (h *customerGrpcHandler) DeleteById(ctx context.Context, req *proto.DeleteCustomerByIdRequest) (*emptypb.Empty, error) {
	if err := h.customerSvc.DeleteById(ctx, req.Id); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *customerGrpcHandler) customerResponse(c *customer.Customer) *proto.CustomerResponse {
	return &proto.CustomerResponse{
		Id:         c.Id,
		FirstName:  c.FirstName,
		LastName:   c.LastName,
		MiddleName: c.MiddleName,
		Email:      c.Email,
		Importance: proto.CustomerImportance(c.Importance),
		Inactive:   c.Inactive,
	}
}
