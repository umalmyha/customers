package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/umalmyha/customers/internal/repository"
)

type CustomerService interface {
	FindAll(context.Context) ([]*customer.Customer, error)
	FindById(context.Context, string) (*customer.Customer, error)
	Create(context.Context, *customer.Customer) (*customer.Customer, error)
	DeleteById(context.Context, string) error
	Upsert(context.Context, *customer.Customer) (*customer.Customer, error)
}

type customerService struct {
	customerRps repository.CustomerRepository
}

func NewCustomerService(customerRps repository.CustomerRepository) CustomerService {
	return &customerService{customerRps: customerRps}
}

func (s *customerService) Create(ctx context.Context, c *customer.Customer) (*customer.Customer, error) {
	c.Id = uuid.NewString()
	if err := s.customerRps.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *customerService) DeleteById(ctx context.Context, id string) error {
	return s.customerRps.DeleteById(ctx, id)
}

func (s *customerService) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	return s.customerRps.FindById(ctx, id)
}

func (s *customerService) FindAll(ctx context.Context) ([]*customer.Customer, error) {
	return s.customerRps.FindAll(ctx)
}

func (s *customerService) Upsert(ctx context.Context, c *customer.Customer) (*customer.Customer, error) {
	existingCustomer, err := s.customerRps.FindById(ctx, c.Id)
	if err != nil {
		return nil, err
	}

	if existingCustomer == nil {
		if err := s.customerRps.Create(ctx, c); err != nil {
			return nil, err
		}
		return c, nil
	}

	if err := s.customerRps.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}
