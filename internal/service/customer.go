package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/customers/internal/cache"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/repository"
)

// CustomerService represents behavior of customer service
type CustomerService interface {
	FindAll(context.Context) ([]*model.Customer, error)
	FindByID(context.Context, string) (*model.Customer, error)
	Create(context.Context, *model.Customer) (*model.Customer, error)
	DeleteByID(context.Context, string) error
	Upsert(context.Context, *model.Customer) (*model.Customer, error)
}

type customerService struct {
	customerRps repository.CustomerRepository
	cacheRps    cache.CustomerCacheRepository
}

// NewCustomerService builds new customerService
func NewCustomerService(
	customerRps repository.CustomerRepository,
	cacheRps cache.CustomerCacheRepository,
) CustomerService {
	return &customerService{customerRps: customerRps, cacheRps: cacheRps}
}

func (s *customerService) Create(ctx context.Context, c *model.Customer) (*model.Customer, error) {
	c.ID = uuid.NewString()
	if err := s.customerRps.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *customerService) DeleteByID(ctx context.Context, id string) error {
	if err := s.cacheRps.DeleteByID(ctx, id); err != nil {
		return err
	}

	if err := s.customerRps.DeleteByID(ctx, id); err != nil {
		return err
	}
	return nil
}

func (s *customerService) FindByID(ctx context.Context, id string) (*model.Customer, error) {
	c, err := s.cacheRps.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if c != nil {
		return c, nil
	}

	c, err = s.customerRps.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.cacheRps.Create(ctx, c); err != nil {
		return nil, err
	}

	return c, nil
}

func (s *customerService) FindAll(ctx context.Context) ([]*model.Customer, error) {
	customers, err := s.customerRps.FindAll(ctx)
	if err != nil {
		logrus.Errorf("failed to read all customers - %v", err)
		return nil, err
	}
	return customers, nil
}

func (s *customerService) Upsert(ctx context.Context, c *model.Customer) (*model.Customer, error) {
	existingCustomer, err := s.customerRps.FindByID(ctx, c.ID)
	if err != nil {
		return nil, err
	}

	if existingCustomer == nil {
		if err := s.customerRps.Create(ctx, c); err != nil {
			return nil, err
		}
		return c, nil
	}

	if err := s.cacheRps.DeleteByID(ctx, c.ID); err != nil {
		return nil, err
	}

	if err := s.customerRps.Update(ctx, c); err != nil {
		return nil, err
	}

	return c, nil
}
