package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	cacheMocks "github.com/umalmyha/customers/internal/cache/mocks"
	"github.com/umalmyha/customers/internal/model"
	rpsMocks "github.com/umalmyha/customers/internal/repository/mocks"
)

type customerTestData struct {
	ctx      context.Context
	customer *model.Customer
}

type customerServiceTestSuite struct {
	suite.Suite
	customerSvc       CustomerService
	customerRpsMock   *rpsMocks.CustomerRepository
	customerCacheMock *cacheMocks.CustomerCacheRepository
	testData          *customerTestData
}

func (s *customerServiceTestSuite) SetupSuite() {
	s.testData = &customerTestData{
		ctx: context.Background(),
		customer: &model.Customer{
			ID:         "ecc770d9-4576-4f72-affa-8b1454246692",
			FirstName:  "John",
			LastName:   "Walls",
			MiddleName: nil,
			Email:      "john.walls@somemal.com",
			Importance: model.ImportanceCritical,
			Inactive:   false,
		},
	}
}

func (s *customerServiceTestSuite) SetupTest() {
	t := s.T()
	s.customerRpsMock = rpsMocks.NewCustomerRepository(t)
	s.customerCacheMock = cacheMocks.NewCustomerCacheRepository(t)
	s.customerSvc = NewCustomerService(s.customerRpsMock, s.customerCacheMock)
}

func (s *customerServiceTestSuite) TestFindByIDFromCache() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerCacheMock.On("FindByID", ctx, customer.ID).Return(customer, nil).Once()

	s.T().Log("customer must be found in cache")
	{
		_, err := s.customerSvc.FindByID(ctx, customer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertNotCalled(s.T(), "FindByID", ctx, customer.ID)
	}
}

func (s *customerServiceTestSuite) TestFindByIDNotFound() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerCacheMock.On("FindByID", ctx, customer.ID).Return(nil, nil).Once()
	s.customerRpsMock.On("FindByID", ctx, customer.ID).Return(nil, nil).Once()

	s.T().Log("customer is missing in cache and in primary datasource")
	{
		c, err := s.customerSvc.FindByID(ctx, customer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.Assert().Nil(c, "no customer must be present but it was found")
		s.customerCacheMock.AssertNotCalled(s.T(), "Create", mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestFindByIDCached() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerCacheMock.On("FindByID", ctx, customer.ID).Return(nil, nil).Once()
	s.customerRpsMock.On("FindByID", ctx, customer.ID).Return(customer, nil).Once()
	s.customerCacheMock.On("Create", ctx, customer).Return(nil).Once()

	s.T().Log("customer is not in cache, found in primary datasource and cached")
	{
		c, err := s.customerSvc.FindByID(ctx, customer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.Assert().NotNil(c, "customer must be found")
		s.customerCacheMock.AssertCalled(s.T(), "Create", ctx, mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestDeleteByIDCacheFailed() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerCacheMock.On("DeleteByID", ctx, customer.ID).Return(errors.New("cache err")).Once()

	s.T().Log("delete customer from cache failed")
	{
		err := s.customerSvc.DeleteByID(ctx, customer.ID)
		s.Assert().Error(err, "cache raised error - error must be raised up")
	}
}

func (s *customerServiceTestSuite) TestDeleteByIDSuccessfully() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerCacheMock.On("DeleteByID", ctx, customer.ID).Return(nil).Once()
	s.customerRpsMock.On("DeleteByID", ctx, customer.ID).Return(nil).Once()

	s.T().Log("deleted successfully")
	{
		err := s.customerSvc.DeleteByID(ctx, customer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertCalled(s.T(), "DeleteByID", ctx, customer.ID)
	}
}

func (s *customerServiceTestSuite) TestUpsertNewCustomer() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerRpsMock.On("FindByID", ctx, customer.ID).Return(nil, nil).Once()
	s.customerRpsMock.On("Create", ctx, mock.AnythingOfType("*model.Customer")).Return(nil).Once()

	s.T().Log("user is not present, so must be created")
	{
		_, err := s.customerSvc.Upsert(ctx, customer)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertNotCalled(s.T(), "Update", ctx, mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestUpsertUpdateCustomer() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerRpsMock.On("FindByID", ctx, customer.ID).Return(customer, nil).Once()
	s.customerCacheMock.On("DeleteByID", ctx, customer.ID).Return(nil).Once()
	s.customerRpsMock.On("Update", ctx, mock.AnythingOfType("*model.Customer")).Return(nil).Once()

	s.T().Log("user is present, so must be updated")
	{
		_, err := s.customerSvc.Upsert(ctx, customer)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertNotCalled(s.T(), "Create", ctx, mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestCreateSuccessfully() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	s.customerRpsMock.On("Create", ctx, customer).Return(nil).Once()

	s.T().Log("user must be created successfully")
	{
		_, err := s.customerSvc.Create(ctx, customer)
		s.Assert().NoError(err, "no error must be raised")
	}
}

func (s *customerServiceTestSuite) TestFindAllSuccessfully() {
	ctx := s.testData.ctx
	customer := s.testData.customer

	customers := []*model.Customer{
		customer,
	}

	s.customerRpsMock.On("FindAll", ctx).Return(customers, nil).Once()

	s.T().Log("users must be found from data source")
	{
		_, err := s.customerSvc.FindAll(ctx)
		s.Assert().NoError(err, "no error must be raised")
	}
}

// start customer service test suite
func TestCustomerServiceTestSuite(t *testing.T) {
	suite.Run(t, new(customerServiceTestSuite))
}
