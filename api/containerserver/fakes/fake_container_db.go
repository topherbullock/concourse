// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"github.com/concourse/atc/api/containerserver"
	"github.com/concourse/atc/db"
)

type FakeContainerDB struct {
	GetContainerStub        func(handle string) (db.SavedContainer, bool, error)
	getContainerMutex       sync.RWMutex
	getContainerArgsForCall []struct {
		handle string
	}
	getContainerReturns struct {
		result1 db.SavedContainer
		result2 bool
		result3 error
	}
	FindContainersByDescriptorsStub        func(db.Container) ([]db.SavedContainer, error)
	findContainersByDescriptorsMutex       sync.RWMutex
	findContainersByDescriptorsArgsForCall []struct {
		arg1 db.Container
	}
	findContainersByDescriptorsReturns struct {
		result1 []db.SavedContainer
		result2 error
	}
}

func (fake *FakeContainerDB) GetContainer(handle string) (db.SavedContainer, bool, error) {
	fake.getContainerMutex.Lock()
	fake.getContainerArgsForCall = append(fake.getContainerArgsForCall, struct {
		handle string
	}{handle})
	fake.getContainerMutex.Unlock()
	if fake.GetContainerStub != nil {
		return fake.GetContainerStub(handle)
	} else {
		return fake.getContainerReturns.result1, fake.getContainerReturns.result2, fake.getContainerReturns.result3
	}
}

func (fake *FakeContainerDB) GetContainerCallCount() int {
	fake.getContainerMutex.RLock()
	defer fake.getContainerMutex.RUnlock()
	return len(fake.getContainerArgsForCall)
}

func (fake *FakeContainerDB) GetContainerArgsForCall(i int) string {
	fake.getContainerMutex.RLock()
	defer fake.getContainerMutex.RUnlock()
	return fake.getContainerArgsForCall[i].handle
}

func (fake *FakeContainerDB) GetContainerReturns(result1 db.SavedContainer, result2 bool, result3 error) {
	fake.GetContainerStub = nil
	fake.getContainerReturns = struct {
		result1 db.SavedContainer
		result2 bool
		result3 error
	}{result1, result2, result3}
}

func (fake *FakeContainerDB) FindContainersByDescriptors(arg1 db.Container) ([]db.SavedContainer, error) {
	fake.findContainersByDescriptorsMutex.Lock()
	fake.findContainersByDescriptorsArgsForCall = append(fake.findContainersByDescriptorsArgsForCall, struct {
		arg1 db.Container
	}{arg1})
	fake.findContainersByDescriptorsMutex.Unlock()
	if fake.FindContainersByDescriptorsStub != nil {
		return fake.FindContainersByDescriptorsStub(arg1)
	} else {
		return fake.findContainersByDescriptorsReturns.result1, fake.findContainersByDescriptorsReturns.result2
	}
}

func (fake *FakeContainerDB) FindContainersByDescriptorsCallCount() int {
	fake.findContainersByDescriptorsMutex.RLock()
	defer fake.findContainersByDescriptorsMutex.RUnlock()
	return len(fake.findContainersByDescriptorsArgsForCall)
}

func (fake *FakeContainerDB) FindContainersByDescriptorsArgsForCall(i int) db.Container {
	fake.findContainersByDescriptorsMutex.RLock()
	defer fake.findContainersByDescriptorsMutex.RUnlock()
	return fake.findContainersByDescriptorsArgsForCall[i].arg1
}

func (fake *FakeContainerDB) FindContainersByDescriptorsReturns(result1 []db.SavedContainer, result2 error) {
	fake.FindContainersByDescriptorsStub = nil
	fake.findContainersByDescriptorsReturns = struct {
		result1 []db.SavedContainer
		result2 error
	}{result1, result2}
}

var _ containerserver.ContainerDB = new(FakeContainerDB)
