package db_test

import (
	"time"

	"github.com/cloudfoundry/bosh-cli/director/template"
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/creds"
	"github.com/concourse/concourse/atc/db"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resource Config Scope", func() {
	var resourceScope db.ResourceConfigScope

	BeforeEach(func() {
		setupTx, err := dbConn.Begin()
		Expect(err).ToNot(HaveOccurred())

		brt := db.BaseResourceType{
			Name: "some-type",
		}
		_, err = brt.FindOrCreate(setupTx)
		Expect(err).NotTo(HaveOccurred())
		Expect(setupTx.Commit()).To(Succeed())

		pipeline, _, err := defaultTeam.SavePipeline("scope-pipeline", atc.Config{
			Resources: atc.ResourceConfigs{
				{
					Name: "some-resource",
					Type: "some-type",
					Source: atc.Source{
						"some": "source",
					},
				},
			},
		}, db.ConfigVersion(0), db.PipelineUnpaused)
		Expect(err).NotTo(HaveOccurred())

		resource, found, err := pipeline.Resource("some-resource")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		resourceScope, err = resource.SetResourceConfig(logger, atc.Source{"some": "source"}, creds.VersionedResourceTypes{})
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("SaveVersions", func() {
		var (
			originalVersionSlice []atc.Version
		)

		BeforeEach(func() {
			originalVersionSlice = []atc.Version{
				{"ref": "v1"},
				{"ref": "v3"},
			}
		})

		// XXX: Can make test more resilient if there is a method that gives all versions by descending check order
		It("ensures versioned resources have the correct check_order", func() {
			err := resourceScope.SaveVersions(originalVersionSlice)
			Expect(err).ToNot(HaveOccurred())

			latestVR, found, err := resourceScope.LatestVersion()
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())

			Expect(latestVR.Version()).To(Equal(db.Version{"ref": "v3"}))
			Expect(latestVR.CheckOrder()).To(Equal(2))

			pretendCheckResults := []atc.Version{
				{"ref": "v2"},
				{"ref": "v3"},
			}

			err = resourceScope.SaveVersions(pretendCheckResults)
			Expect(err).ToNot(HaveOccurred())

			latestVR, found, err = resourceScope.LatestVersion()
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())

			Expect(latestVR.Version()).To(Equal(db.Version{"ref": "v3"}))
			Expect(latestVR.CheckOrder()).To(Equal(4))
		})

		Context("when the versions already exists", func() {
			var newVersionSlice []atc.Version

			BeforeEach(func() {
				newVersionSlice = []atc.Version{
					{"ref": "v1"},
					{"ref": "v3"},
				}
			})

			It("does not change the check order", func() {
				err := resourceScope.SaveVersions(newVersionSlice)
				Expect(err).ToNot(HaveOccurred())

				latestVR, found, err := resourceScope.LatestVersion()
				Expect(err).ToNot(HaveOccurred())
				Expect(found).To(BeTrue())

				Expect(latestVR.Version()).To(Equal(db.Version{"ref": "v3"}))
				Expect(latestVR.CheckOrder()).To(Equal(2))
			})
		})
	})

	Describe("LatestVersion", func() {
		Context("when the resource config exists", func() {
			var latestCV db.ResourceConfigVersion

			BeforeEach(func() {
				originalVersionSlice := []atc.Version{
					{"ref": "v1"},
					{"ref": "v3"},
				}

				err := resourceScope.SaveVersions(originalVersionSlice)
				Expect(err).ToNot(HaveOccurred())

				var found bool
				latestCV, found, err = resourceScope.LatestVersion()
				Expect(err).ToNot(HaveOccurred())
				Expect(found).To(BeTrue())
			})

			It("gets latest version of resource", func() {
				Expect(latestCV.Version()).To(Equal(db.Version{"ref": "v3"}))
				Expect(latestCV.CheckOrder()).To(Equal(2))
			})
		})
	})

	Describe("FindVersion", func() {
		BeforeEach(func() {
			originalVersionSlice := []atc.Version{
				{"ref": "v1"},
				{"ref": "v3"},
			}

			err := resourceScope.SaveVersions(originalVersionSlice)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when the version exists", func() {
			var latestCV db.ResourceConfigVersion

			BeforeEach(func() {
				var err error
				var found bool
				latestCV, found, err = resourceScope.FindVersion(atc.Version{"ref": "v1"})
				Expect(err).ToNot(HaveOccurred())
				Expect(found).To(BeTrue())
			})

			It("gets the version of resource", func() {
				Expect(latestCV.ResourceConfigScope().ID()).To(Equal(resourceScope.ID()))
				Expect(latestCV.Version()).To(Equal(db.Version{"ref": "v1"}))
				Expect(latestCV.CheckOrder()).To(Equal(1))
			})
		})

		Context("when the version does not exist", func() {
			var found bool
			var latestCV db.ResourceConfigVersion

			BeforeEach(func() {
				var err error
				latestCV, found, err = resourceScope.FindVersion(atc.Version{"ref": "v2"})
				Expect(err).ToNot(HaveOccurred())
			})

			It("does not get the version of resource", func() {
				Expect(found).To(BeFalse())
				Expect(latestCV).To(BeNil())
			})
		})
	})

	Describe("AcquireResourceCheckingLock", func() {
		var (
			someResource        db.Resource
			resourceConfigScope db.ResourceConfigScope
		)

		BeforeEach(func() {
			var err error
			var found bool

			someResource, found, err = defaultPipeline.Resource("some-resource")
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())

			pipelineResourceTypes, err := defaultPipeline.ResourceTypes()
			Expect(err).ToNot(HaveOccurred())

			resourceConfigScope, err = someResource.SetResourceConfig(
				logger,
				someResource.Source(),
				creds.NewVersionedResourceTypes(template.StaticVariables{}, pipelineResourceTypes.Deserialize()),
			)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when there has been a check recently", func() {
			Context("when acquiring immediately", func() {
				It("gets the lock", func() {
					lock, acquired, err := resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())

					lock, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when not acquiring immediately", func() {
				It("does not get the lock", func() {
					lock, acquired, err := resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())

					_, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeFalse())
				})
			})
		})

		Context("when there has not been a check recently", func() {
			Context("when acquiring immediately", func() {
				It("gets and keeps the lock and stops others from periodically getting it", func() {
					lock, acquired, err := resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					Consistently(func() bool {
						_, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
						Expect(err).ToNot(HaveOccurred())

						return acquired
					}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())

					time.Sleep(time.Second)

					lock, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())
				})

				It("gets and keeps the lock and stops others from immediately getting it", func() {
					lock, acquired, err := resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					Consistently(func() bool {
						_, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
						Expect(err).ToNot(HaveOccurred())

						return acquired
					}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())

					time.Sleep(time.Second)

					lock, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when not acquiring immediately", func() {
				It("gets and keeps the lock and stops others from periodically getting it", func() {
					lock, acquired, err := resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					Consistently(func() bool {
						_, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
						Expect(err).ToNot(HaveOccurred())

						return acquired
					}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())

					time.Sleep(time.Second)

					lock, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())
				})

				It("gets and keeps the lock and stops others from immediately getting it", func() {
					lock, acquired, err := resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)

					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					Consistently(func() bool {
						_, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, true)
						Expect(err).ToNot(HaveOccurred())

						return acquired
					}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())

					time.Sleep(time.Second)

					lock, acquired, err = resourceConfigScope.AcquireResourceCheckingLock(logger, 1*time.Second, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(acquired).To(BeTrue())

					err = lock.Release()
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})

})
