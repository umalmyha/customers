package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
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
	logger      logrus.FieldLogger
}

func NewCustomerService(customerRps repository.CustomerRepository, logger logrus.FieldLogger) CustomerService {
	return &customerService{customerRps: customerRps, logger: logger}
}

func (s *customerService) Create(ctx context.Context, c *customer.Customer) (*customer.Customer, error) {
	c.Id = uuid.NewString()
	if err := s.customerRps.Create(ctx, c); err != nil {
		s.logger.Errorf("failed to create customer - %v", err)
		return nil, err
	}
	return c, nil
}

func (s *customerService) DeleteById(ctx context.Context, id string) error {
	if err := s.customerRps.DeleteById(ctx, id); err != nil {
		s.logger.Errorf("failed to delete customer with id %s - %v", id, err)
		return err
	}
	return nil
}

func (s *customerService) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	c, err := s.customerRps.FindById(ctx, id)
	if err != nil {
		s.logger.Errorf("failed to read customer with id %s - %v", id, err)
		return nil, err
	}
	return c, nil
}

func (s *customerService) FindAll(ctx context.Context) ([]*customer.Customer, error) {
	customers, err := s.customerRps.FindAll(ctx)
	if err != nil {
		s.logger.Errorf("failed to read all customers - %v", err)
		return nil, err
	}
	return customers, nil
}

func (s *customerService) Upsert(ctx context.Context, c *customer.Customer) (*customer.Customer, error) {
	existingCustomer, err := s.customerRps.FindById(ctx, c.Id)
	if err != nil {
		s.logger.Errorf("failed to read customer with id %s - %v", c.Id, err)
		return nil, err
	}

	if existingCustomer == nil {
		s.logger.Infof("customer with id %s doesn't exist, creating...", c.Id)
		if err := s.customerRps.Create(ctx, c); err != nil {
			return nil, err
		}
		return c, nil
	}

	if err := s.customerRps.Update(ctx, c); err != nil {
		s.logger.Errorf("failed to update customer with id %s - %v", c.Id, err)
		return nil, err
	}
	return c, nil
}
