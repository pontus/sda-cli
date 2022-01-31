package helpers

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HelperTests struct {
	suite.Suite
	tempDir  string
	testFile *os.File
}

func TestHelpersTestSuite(t *testing.T) {
	suite.Run(t, new(HelperTests))
}

func (suite *HelperTests) SetupTest() {

	var err error

	// Create a temporary directory for our files
	suite.tempDir, err = ioutil.TempDir(os.TempDir(), "sda-cli-test-")
	if err != nil {
		log.Fatal("Couldn't create temporary test directory", err)
	}

	// create an existing test file with some known content
	suite.testFile, err = ioutil.TempFile(suite.tempDir, "testfile-")
	if err != nil {
		log.Fatal("cannot create temporary public key file", err)
	}

	err = ioutil.WriteFile(suite.testFile.Name(), []byte("content"), 0600)
	if err != nil {
		log.Fatalf("failed to write to testfile: %s", err)
	}
}

func (suite *HelperTests) TearDownTest() {
	os.Remove(suite.testFile.Name())
	os.Remove(suite.tempDir)
}

func (suite *HelperTests) TestFileExists() {
	// file exists
	testExists := FileExists(suite.testFile.Name())
	suite.Equal(testExists, true)
	// file does not exists
	testMissing := FileExists("does-not-exist")
	suite.Equal(testMissing, false)
	// file is a directory
	testIsDir := FileExists(suite.tempDir)
	suite.Equal(testIsDir, true)
}

func (suite *HelperTests) TestFileIsReadable() {
	// file doesn't exist
	testMissing := FileIsReadable("does-not-exist")
	suite.Equal(testMissing, false)

	// file is a directory
	testIsDir := FileIsReadable(suite.tempDir)
	suite.Equal(testIsDir, false)

	// file can be read
	testFileOk := FileIsReadable(suite.testFile.Name())
	suite.Equal(testFileOk, true)

	// test file permissions. This doesn't work on windows, so we do an extra
	// check to see if this test makes sense.
	if runtime.GOOS != "windows" {
		err := os.Chmod(suite.testFile.Name(), 0000)
		if err != nil {
			log.Fatal("Couldn't set file permissions of test file")
		}
		// file permissions don't allow reading
		testDisallowed := FileIsReadable(suite.testFile.Name())
		suite.Equal(testDisallowed, false)

		// restore permissions
		err = os.Chmod(suite.testFile.Name(), 0600)
		if err != nil {
			log.Fatal("Couldn't restore file permissions of test file")
		}
	}
}

func (suite *HelperTests) TestFormatSubcommandUsage() {
	// check formatting of malformed usage strings without %s for os.Args[0]
	malformed_no_format_string := "USAGE: do that stuff"
	test_missing_args_format := FormatSubcommandUsage(malformed_no_format_string)
	suite.Equal(malformed_no_format_string, test_missing_args_format)

	// check formatting when the USAGE string is missing
	malformed_no_usage := `module: this module does all the fancies stuff,
								   and virtually none of the non-fancy stuff.
								   run with: %s module`
	test_no_usage := FormatSubcommandUsage(malformed_no_usage)
	suite.Equal(fmt.Sprintf(malformed_no_usage, os.Args[0]), test_no_usage)

	// check formatting when the usage string is correctly formatted

	correct_usage := `USAGE: %s module <args>

module: this module does all the fancies stuff,
        and virtually none of the non-fancy stuff.`

	correct_format := fmt.Sprintf(`
module: this module does all the fancies stuff,
        and virtually none of the non-fancy stuff.

        USAGE: %s module <args>

`, os.Args[0])
	test_correct := FormatSubcommandUsage(correct_usage)
	suite.Equal(correct_format, test_correct)

}
