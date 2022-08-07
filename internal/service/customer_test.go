package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	cacheMocks "github.com/umalmyha/customers/internal/cache/mocks"
	"github.com/umalmyha/customers/internal/model"
	rpsMocks "github.com/umalmyha/customers/internal/repository/mocks"
	"testing"
)

var testCustomerCtx = context.Background()

var testCustomer = &model.Customer{
	ID:         "ecc770d9-4576-4f72-affa-8b1454246692",
	FirstName:  "John",
	LastName:   "Walls",
	MiddleName: nil,
	Email:      "john.walls@somemal.com",
	Importance: model.ImportanceCritical,
	Inactive:   false,
}

type customerServiceTestSuite struct {
	suite.Suite
	customerSvc       CustomerService
	customerRpsMock   *rpsMocks.CustomerRepository
	customerCacheMock *cacheMocks.CustomerCacheRepository
}

func (s *customerServiceTestSuite) SetupTest() {
	t := s.T()
	s.customerRpsMock = rpsMocks.NewCustomerRepository(t)
	s.customerCacheMock = cacheMocks.NewCustomerCacheRepository(t)
	s.customerSvc = NewCustomerService(s.customerRpsMock, s.customerCacheMock)
}

func (s *customerServiceTestSuite) TestFindByIDFromCache() {
	s.customerCacheMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(testCustomer, nil).Once()

	s.T().Log("customer must be found in cache")
	{
		_, err := s.customerSvc.FindByID(testCustomerCtx, testCustomer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertNotCalled(s.T(), "FindByID", testCustomerCtx, testCustomer.ID)
	}
}

func (s *customerServiceTestSuite) TestFindByIDNotFound() {
	s.customerCacheMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(nil, nil).Once()
	s.customerRpsMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(nil, nil).Once()

	s.T().Log("customer is missing in cache and in primary datasource")
	{
		customer, err := s.customerSvc.FindByID(testCustomerCtx, testCustomer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.Assert().Nil(customer, "no customer must be present but it was found")
		s.customerCacheMock.AssertNotCalled(s.T(), "Create", mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestFindByIDCached() {
	s.customerCacheMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(nil, nil).Once()
	s.customerRpsMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(testCustomer, nil).Once()
	s.customerCacheMock.On("Create", testCustomerCtx, testCustomer).Return(nil).Once()

	s.T().Log("customer is not in cache, found in primary datasource and cached")
	{
		customer, err := s.customerSvc.FindByID(testCustomerCtx, testCustomer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.Assert().NotNil(customer, "customer must be found")
		s.customerCacheMock.AssertCalled(s.T(), "Create", testCustomerCtx, mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestDeleteByIDCacheFailed() {
	s.customerCacheMock.On("DeleteByID", testCustomerCtx, testCustomer.ID).Return(errors.New("cache err")).Once()

	s.T().Log("delete customer from cache failed")
	{
		err := s.customerSvc.DeleteByID(testCustomerCtx, testCustomer.ID)
		s.Assert().Error(err, "cache raised error - error must be raised up")
	}
}

func (s *customerServiceTestSuite) TestDeleteByIDSuccessfully() {
	s.customerCacheMock.On("DeleteByID", testCustomerCtx, testCustomer.ID).Return(nil).Once()
	s.customerRpsMock.On("DeleteByID", testCustomerCtx, testCustomer.ID).Return(nil).Once()

	s.T().Log("deleted successfully")
	{
		err := s.customerSvc.DeleteByID(testCustomerCtx, testCustomer.ID)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertCalled(s.T(), "DeleteByID", testCustomerCtx, testCustomer.ID)
	}
}

func (s *customerServiceTestSuite) TestUpsertNewCustomer() {
	s.customerRpsMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(nil, nil).Once()
	s.customerRpsMock.On("Create", testCustomerCtx, mock.AnythingOfType("*model.Customer")).Return(nil).Once()

	s.T().Log("user is not present, so must be created")
	{
		_, err := s.customerSvc.Upsert(testCustomerCtx, testCustomer)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertNotCalled(s.T(), "Update", testCustomerCtx, mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestUpsertUpdateCustomer() {
	s.customerRpsMock.On("FindByID", testCustomerCtx, testCustomer.ID).Return(testCustomer, nil).Once()
	s.customerCacheMock.On("DeleteByID", testCustomerCtx, testCustomer.ID).Return(nil).Once()
	s.customerRpsMock.On("Update", testCustomerCtx, mock.AnythingOfType("*model.Customer")).Return(nil).Once()

	s.T().Log("user is present, so must be updated")
	{
		_, err := s.customerSvc.Upsert(testCustomerCtx, testCustomer)
		s.Assert().NoError(err, "no error must be raised")
		s.customerRpsMock.AssertNotCalled(s.T(), "Create", testCustomerCtx, mock.AnythingOfType("*model.Customer"))
	}
}

func (s *customerServiceTestSuite) TestCreateSuccessfully() {
	s.customerRpsMock.On("Create", testCustomerCtx, testCustomer).Return(nil).Once()

	s.T().Log("user must be created successfully")
	{
		_, err := s.customerSvc.Create(testCustomerCtx, testCustomer)
		s.Assert().NoError(err, "no error must be raised")
	}
}

func (s *customerServiceTestSuite) TestFindAllSuccessfully() {
	customers := []*model.Customer{
		testCustomer,
	}

	s.customerRpsMock.On("FindAll", testCustomerCtx).Return(customers, nil).Once()

	s.T().Log("users must be found from data source")
	{
		_, err := s.customerSvc.FindAll(testCustomerCtx)
		s.Assert().NoError(err, "no error must be raised")
	}
}

// start customer service test suite
func TestCustomerServiceTestSuite(t *testing.T) {
	suite.Run(t, new(customerServiceTestSuite))
}
