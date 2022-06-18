package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/umalmyha/customers/internal/customer"
	"github.com/umalmyha/customers/internal/errors"
	"github.com/umalmyha/customers/internal/repository"
)

var CustomerNotFoundErr = func(id string) error {
	return errors.NewEntryNotFoundErr(fmt.Sprintf("customer with id %s doesn't exist", id))
}

var IncorrectUuidFormatErr = func(id string) error {
	return errors.NewBusinessErr("id", fmt.Sprintf("provided id %s has wrong UUID format", id))
}

type CustomerService interface {
	FindAll(context.Context) ([]customer.Customer, error)
	FindById(context.Context, string) (customer.Customer, error)
	Create(context.Context, customer.NewCustomer) (customer.Customer, error)
	DeleteById(context.Context, string) error
	Upsert(context.Context, customer.UpdateCustomer) (customer.Customer, error)
	Merge(context.Context, customer.PatchCustomer) (customer.Customer, error)
}

type customerService struct {
	customerRepo repository.CustomerRepository
}

func NewCustomerService(customerRepo repository.CustomerRepository) CustomerService {
	return &customerService{customerRepo: customerRepo}
}

func (srv *customerService) Create(ctx context.Context, newCust customer.NewCustomer) (customer.Customer, error) {
	c := customer.Customer{
		Id:         uuid.NewString(),
		FirstName:  newCust.FirstName,
		LastName:   newCust.LastName,
		MiddleName: newCust.MiddleName,
		Email:      newCust.Email,
		Importance: newCust.Importance,
		Inactive:   newCust.Inactive,
	}

	if _, err := srv.customerRepo.Create(ctx, c); err != nil {
		return c, err
	}
	return c, nil
}

func (srv *customerService) DeleteById(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return IncorrectUuidFormatErr(id)
	}

	rmv, err := srv.customerRepo.DeleteById(ctx, id)
	if err != nil {
		return err
	}

	if !rmv {
		return CustomerNotFoundErr(id)
	}
	return nil
}

func (srv *customerService) FindById(ctx context.Context, id string) (customer.Customer, error) {
	if _, err := uuid.Parse(id); err != nil {
		return customer.Customer{}, IncorrectUuidFormatErr(id)
	}

	cust, err := srv.customerRepo.FindById(ctx, id)
	if err != nil {
		return cust, err
	}

	if cust.IsInitial() {
		return cust, CustomerNotFoundErr(id)
	}
	return cust, nil
}

func (srv *customerService) FindAll(ctx context.Context) ([]customer.Customer, error) {
	customers, err := srv.customerRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return customers, nil
}

func (srv *customerService) Upsert(ctx context.Context, updCust customer.UpdateCustomer) (customer.Customer, error) {
	if _, err := uuid.Parse(updCust.Id); err != nil {
		return customer.Customer{}, IncorrectUuidFormatErr(updCust.Id)
	}

	existCust, err := srv.customerRepo.FindById(ctx, updCust.Id)
	if err != nil {
		return existCust, err
	}

	cust := customer.Customer(updCust)
	if existCust.IsInitial() {
		if _, err := srv.customerRepo.Create(ctx, cust); err != nil {
			return customer.Customer{}, err
		}
		return cust, nil
	}

	if _, err := srv.customerRepo.Update(ctx, cust); err != nil {
		return cust, err
	}
	return cust, nil
}

func (srv *customerService) Merge(ctx context.Context, patchCust customer.PatchCustomer) (customer.Customer, error) {
	if _, err := uuid.Parse(patchCust.Id); err != nil {
		return customer.Customer{}, IncorrectUuidFormatErr(patchCust.Id)
	}

	existCust, err := srv.FindById(ctx, patchCust.Id)
	if err != nil {
		return existCust, err
	}

	if existCust.IsInitial() {
		return existCust, CustomerNotFoundErr(patchCust.Id)
	}

	cust := existCust.MergePatch(patchCust)
	if _, err := srv.customerRepo.Update(ctx, cust); err != nil {
		return cust, err
	}
	return cust, nil
}
