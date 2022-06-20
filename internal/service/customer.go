package service

import (
	"context"
	"github.com/umalmyha/customers/internal/customer"
	"github.com/umalmyha/customers/internal/repository"
)

type CustomerService interface {
	FindAll(context.Context) ([]customer.Customer, error)
	FindById(context.Context, string) (customer.Customer, error)
	Create(context.Context, customer.Customer) (customer.Customer, error)
	DeleteById(context.Context, string) error
	Upsert(context.Context, customer.Customer) (customer.Customer, error)
}

type customerService struct {
	customerRepo repository.CustomerRepository
}

func NewCustomerService(customerRepo repository.CustomerRepository) CustomerService {
	return &customerService{customerRepo: customerRepo}
}

func (s *customerService) Create(ctx context.Context, c customer.Customer) (customer.Customer, error) {
	id, err := s.customerRepo.Create(ctx, c)
	if err != nil {
		return c, err
	}

	c.Id = id
	return c, nil
}

func (s *customerService) DeleteById(ctx context.Context, id string) error {
	err := s.customerRepo.DeleteById(ctx, id)
	if err != nil {
		return err
	}
	return nil
}

func (s *customerService) FindById(ctx context.Context, id string) (customer.Customer, error) {
	c, err := s.customerRepo.FindById(ctx, id)
	if err != nil {
		return c, err
	}
	return c, nil
}

func (s *customerService) FindAll(ctx context.Context) ([]customer.Customer, error) {
	customers, err := s.customerRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return customers, nil
}

func (s *customerService) Upsert(ctx context.Context, c customer.Customer) (customer.Customer, error) {
	existCust, err := s.customerRepo.FindById(ctx, c.Id)
	if err != nil {
		return customer.Customer{}, err
	}

	if existCust.Id == "" {
		if _, err := s.customerRepo.Create(ctx, c); err != nil {
			return customer.Customer{}, err
		}
		return c, nil
	}

	if err := s.customerRepo.Update(ctx, c); err != nil {
		return customer.Customer{}, err
	}
	return c, nil
}
