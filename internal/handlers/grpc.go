package handlers

import (
	"context"
	"time"

	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AuthGrpcHandler is gRPC handler for auth endpoint
type AuthGrpcHandler struct {
	proto.UnimplementedAuthServiceServer
	authSvc service.AuthService
}

// NewAuthGrpcHandler builds new AuthGrpcHandler
func NewAuthGrpcHandler(authSvc service.AuthService) *AuthGrpcHandler {
	return &AuthGrpcHandler{
		UnimplementedAuthServiceServer: proto.UnimplementedAuthServiceServer{},
		authSvc:                        authSvc,
	}
}

// Signup sing up user
func (h *AuthGrpcHandler) Signup(ctx context.Context, req *proto.SignupRequest) (*proto.NewUserResponse, error) {
	u, err := h.authSvc.Signup(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}

	return &proto.NewUserResponse{
		Id:    u.ID,
		Email: u.Email,
	}, nil
}

// Login logins user
func (h *AuthGrpcHandler) Login(ctx context.Context, req *proto.LoginRequest) (*proto.SessionResponse, error) {
	jwt, rfrToken, err := h.authSvc.Login(ctx, req.Email, req.Password, req.Fingerprint, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	return &proto.SessionResponse{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.ID,
	}, nil
}

// Logout logouts user
func (h *AuthGrpcHandler) Logout(ctx context.Context, req *proto.LogoutRequest) (*emptypb.Empty, error) {
	if err := h.authSvc.Logout(ctx, req.RefreshToken); err != nil {
		return nil, err
	}
	return new(emptypb.Empty), nil
}

// Refresh refreshes user session
func (h *AuthGrpcHandler) Refresh(ctx context.Context, req *proto.RefreshRequest) (*proto.SessionResponse, error) {
	jwt, rfrToken, err := h.authSvc.Refresh(ctx, req.RefreshToken, req.Fingerprint, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	return &proto.SessionResponse{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.ID,
	}, nil
}

// CustomerGrpcHandler is gRPC handler for customers endpoint
type CustomerGrpcHandler struct {
	proto.UnimplementedCustomerServiceServer
	customerSvc service.CustomerService
}

// NewCustomerGrpcHandler builds customerGrpcHandler
func NewCustomerGrpcHandler(customerSvc service.CustomerService) *CustomerGrpcHandler {
	return &CustomerGrpcHandler{
		UnimplementedCustomerServiceServer: proto.UnimplementedCustomerServiceServer{},
		customerSvc:                        customerSvc,
	}
}

// GetByID get customer by id
func (h *CustomerGrpcHandler) GetByID(ctx context.Context, req *proto.GetCustomerByIdRequest) (*proto.CustomerResponse, error) {
	c, err := h.customerSvc.FindByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return h.customerResponse(c), nil
}

// GetAll get all customers
func (h *CustomerGrpcHandler) GetAll(ctx context.Context, _ *emptypb.Empty) (*proto.CustomerListResponse, error) {
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

// Create creates new customer
func (h *CustomerGrpcHandler) Create(ctx context.Context, req *proto.NewCustomerRequest) (*proto.CustomerResponse, error) {
	c, err := h.customerSvc.Create(ctx, &model.Customer{
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		MiddleName: req.MiddleName,
		Email:      req.Email,
		Importance: model.Importance(req.Importance),
		Inactive:   req.Inactive,
	})
	if err != nil {
		return nil, err
	}

	return h.customerResponse(c), nil
}

// Upsert create/update customer
func (h *CustomerGrpcHandler) Upsert(ctx context.Context, req *proto.UpdateCustomerRequest) (*proto.CustomerResponse, error) {
	c, err := h.customerSvc.Upsert(ctx, &model.Customer{
		ID:         req.Id,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		MiddleName: req.MiddleName,
		Email:      req.Email,
		Importance: model.Importance(req.Importance),
		Inactive:   req.Inactive,
	})
	if err != nil {
		return nil, err
	}

	return h.customerResponse(c), nil
}

// DeleteByID deletes customer by id
func (h *CustomerGrpcHandler) DeleteByID(ctx context.Context, req *proto.DeleteCustomerByIdRequest) (*emptypb.Empty, error) {
	if err := h.customerSvc.DeleteByID(ctx, req.Id); err != nil {
		return nil, err
	}
	return new(emptypb.Empty), nil
}

func (h *CustomerGrpcHandler) customerResponse(c *model.Customer) *proto.CustomerResponse {
	return &proto.CustomerResponse{
		Id:         c.ID,
		FirstName:  c.FirstName,
		LastName:   c.LastName,
		MiddleName: c.MiddleName,
		Email:      c.Email,
		Importance: proto.CustomerImportance(c.Importance),
		Inactive:   c.Inactive,
	}
}
