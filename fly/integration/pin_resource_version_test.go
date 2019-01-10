package integration_test

import (
	"github.com/onsi/gomega/gbytes"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Fly CLI", func() {
	Describe("pin-resource-version", func() {
		Context("make sure the command exists", func() {
			FIt("calls the pin-resource-version command", func() {
				flyCmd := exec.Command(flyPath, "pin-resource-version")
				sess, err := gexec.Start(flyCmd, GinkgoWriter, GinkgoWriter)

				Expect(err).ToNot(HaveOccurred())
				Consistently(sess.Err).ShouldNot(gbytes.Say("error: Unknown command"))

				<-sess.Exited
			})
		})
	})
})
