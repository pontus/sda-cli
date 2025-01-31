package helpers

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang-jwt/jwt"
	"github.com/manifoldco/promptui"
	"github.com/neicnordic/crypt4gh/keys"
	log "github.com/sirupsen/logrus"
	"github.com/vbauerster/mpb/v8"
	"golang.org/x/exp/slices"
	"gopkg.in/ini.v1"
)

//
// Helper functions used by more than one module
//

// FileExists checks if a file exists in the file system. Note that this
// function will not check if the file is readable, or if the file is a
// directory, only if it exists.
func FileExists(filename string) bool {
	_, err := os.Stat(filename)

	return err == nil
}

// FileIsReadable checks that a file exists, and is readable by the program.
func FileIsReadable(filename string) bool {
	fileInfo, err := os.Stat(filename)
	if err != nil || fileInfo.IsDir() {
		return false
	}
	// Check readability by simply trying to open the file and read one byte
	inFile, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return false
	}
	defer func() {
		if err := inFile.Close(); err != nil {
			log.Errorf("Error closing file: %s\n", err)
		}
	}()

	test := make([]byte, 1)
	_, err = inFile.Read(test)

	return err == nil
}

// FormatSubcommandUsage moves the lines in the standard usage strings around so
// that the usage string is indented under the help text instead of above it.
func FormatSubcommandUsage(usageString string) string {

	// check that there's a formatting thing for os.Args[0]
	if !strings.Contains(usageString, "%s") && !strings.Contains(usageString, "%v") {
		return usageString
	}

	// format usage string with command name
	usageString = fmt.Sprintf(usageString, os.Args[0])

	// break string into lines
	lines := strings.Split(strings.TrimSpace(usageString), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "USAGE:") {
		// if we don't have enough data, just return the usage string as is
		return usageString
	}
	// reformat lines
	usage := lines[0]

	return fmt.Sprintf("\n%s\n\n    %s\n\n", strings.Join(lines[2:], "\n"), usage)
}

// PromptPassword creates a user prompt for inputting passwords, where all
// characters are masked with "*"
func PromptPassword(message string) (password string, err error) {
	prompt := promptui.Prompt{
		Label: message,
		Mask:  '*',
	}

	return prompt.Run()
}

// ParseS3ErrorResponse checks if reader stream is xml encoded and if yes unmarshals
// the xml response and returns it.
func ParseS3ErrorResponse(respBody io.Reader) (string, error) {

	respMsg, err := io.ReadAll(respBody)
	if err != nil {
		return "", fmt.Errorf("failed to read from response body, reason: %v", err)
	}

	if !strings.Contains(string(respMsg), `xml version`) {
		return "", fmt.Errorf("cannot parse response body, reason: not xml")
	}

	xmlErrorResponse := XMLerrorResponse{}
	err = xml.Unmarshal(respMsg, &xmlErrorResponse)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal xml response, reason: %v", err)
	}

	return fmt.Sprintf("%+v", xmlErrorResponse), nil
}

// Removes all positional arguments from args, and returns them.
// This function assumes that all flags have exactly one value.
func getPositional(args []string) ([]string, []string) {
	argList := []string{"-r", "--r", "--force-overwrite", "-force-overwrite", "--force-unencrypted", "-force-unencrypted"}
	i := 1
	var positional []string
	for i < len(args) {
		switch {
		case slices.Contains(argList, args[i]):
			// if the current args is a boolean flag, skip it
			i++
		case args[i][0] == '-':
			// if the current arg is a flag, skip the flag and its value
			i += 2
		default:
			// if the current arg is positional, remove it and add it to
			// `positional`
			positional = append(positional, args[i])
			args = append(args[:i], args[i+1:]...)
		}
	}

	return positional, args
}

func ParseArgs(args []string, argFlags *flag.FlagSet) error {
	var pos []string
	pos, args = getPositional(args)
	// append positional args back at the end of args
	args = append(args, pos...)
	err := argFlags.Parse(args[1:])

	return err
}

//
// shared structs
//

// struct type to keep track of infiles and outfiles for encryption and
// decryption
type EncryptionFileSet struct {
	Unencrypted string
	Encrypted   string
}

// struct type to unmarshall xml error response from s3 server
type XMLerrorResponse struct {
	Code     string `xml:"Code"`
	Message  string `xml:"Message"`
	Resource string `xml:"Resource"`
}

// progress bar definitions
// Produces a progress bar with decorators that can produce different styles
// Check https://github.com/vbauerster/mpb for more info and how to use it
type CustomReader struct {
	Fp      *os.File
	Size    int64
	Reads   int64
	Bar     *mpb.Bar
	SignMap map[int64]struct{}
	Mux     sync.Mutex
}

func (r *CustomReader) Read(p []byte) (int, error) {
	return r.Fp.Read(p)
}

func (r *CustomReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.Fp.ReadAt(p, off)
	if err != nil {
		return n, err
	}

	r.Bar.SetTotal(r.Size, false)

	r.Mux.Lock()
	// Ignore the first signature call
	if _, ok := r.SignMap[off]; ok {
		r.Reads += int64(n)
		r.Bar.SetCurrent(r.Reads)
	} else {
		r.SignMap[off] = struct{}{}
	}
	r.Mux.Unlock()

	return n, err
}

func (r *CustomReader) Seek(offset int64, whence int) (int64, error) {
	return r.Fp.Seek(offset, whence)
}

// Config struct for storing the s3cmd file values
type Config struct {
	AccessKey            string `ini:"access_key"`
	SecretKey            string `ini:"secret_key"`
	AccessToken          string `ini:"access_token"`
	HostBucket           string `ini:"host_bucket"`
	HostBase             string `ini:"host_base"`
	MultipartChunkSizeMb int64  `ini:"multipart_chunk_size_mb"`
	GuessMimeType        bool   `ini:"guess_mime_type"`
	Encoding             string `ini:"encoding"`
	CheckSslCertificate  bool   `ini:"check_ssl_certificate"`
	CheckSslHostname     bool   `ini:"check_ssl_hostname"`
	UseHTTPS             bool   `ini:"use_https"`
	SocketTimeout        int    `ini:"socket_timeout"`
	HumanReadableSizes   bool   `ini:"human_readable_sizes"`
	PublicKey            string `ini:"public_key"`
}

// LoadConfigFile loads ini configuration file to the Config struct
func LoadConfigFile(path string) (*Config, error) {

	config := &Config{}

	cfg, err := ini.Load(path)
	if err != nil {
		return config, err
	}

	// ini sees a DEFAULT section by default
	var iniSection string
	if len(cfg.SectionStrings()) > 1 {
		iniSection = cfg.SectionStrings()[1]
	} else {
		iniSection = cfg.SectionStrings()[0]
	}

	if err := cfg.Section(iniSection).MapTo(config); err != nil {
		return nil, err
	}

	if config.AccessKey == "" || config.AccessToken == "" {
		return nil, errors.New("failed to find credentials in configuration file")
	}

	if config.HostBase == "" {
		return nil, errors.New("failed to find endpoint in configuration file")
	}

	if config.UseHTTPS {
		config.HostBase = "https://" + config.HostBase
	}

	if config.Encoding == "" {
		config.Encoding = "UTF-8"
	}

	// Where 15 is the default chunk size of the library
	if config.MultipartChunkSizeMb <= 15 {
		config.MultipartChunkSizeMb = 15
	}

	return config, nil
}

// GetAuth calls LoadConfig if we have a config file, otherwise try to load .sda-cli-session
func GetAuth(path string) (*Config, error) {

	if path != "" {
		return LoadConfigFile(path)
	}
	if FileExists(".sda-cli-session") {
		return LoadConfigFile(".sda-cli-session")
	}

	return nil, errors.New("failed to read the configuration file")
}

func GetPublicKey() (string, error) {
	// Check if the ".sda-cli-session" file exists
	if !FileExists(".sda-cli-session") {
		return "", errors.New("configuration file (.sda-cli-session) not found")
	}

	if FileExists(".sda-cli-session") {
		file, err := os.Open(".sda-cli-session")
		if err != nil {
			fmt.Println("could not read file:", file)
		}
	}

	// Load the configuration file
	config, err := LoadConfigFile(".sda-cli-session")
	if err != nil {
		return "", fmt.Errorf("failed to load configuration file: %w", err)
	}

	// Check if the PublicKey field is present in the config
	if config.PublicKey == "" {
		return "", errors.New("public key not found in the configuration")
	}

	// Create a fixed-size array to hold the public key data
	var publicKeyData [32]byte
	b := []byte(config.PublicKey)
	copy(publicKeyData[:], b)

	// Open or create a file named "key-from-oidc.pub.pem" in write-only mode with file permissions 0600
	pubFile, err := os.OpenFile(filepath.Clean("key-from-oidc.pub.pem"), os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open or create the public key file: %w", err)
	}
	defer func() {
		// Close the file and log any error that may occur
		if cerr := pubFile.Close(); cerr != nil {
			log.Errorf("Error closing file: %s\n", cerr)
		}
	}()

	// Write the publicKeyData array to the "key-from-oidc.pub.pem" file in Crypt4GHX25519 public key format
	err = keys.WriteCrypt4GHX25519PublicKey(pubFile, publicKeyData)
	if err != nil {
		return "", fmt.Errorf("failed to write the public key data: %w", err)
	}

	// If everything is successful, return the name of the generated public key file
	return "key-from-oidc.pub.pem", nil
}

// CheckTokenExpiration is used to determine whether the token is expiring in less than a day
func CheckTokenExpiration(accessToken string) (bool, error) {

	// Parse jwt token with unverifies, since we don't need to check the signatures here
	token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
	if err != nil {
		return false, fmt.Errorf("could not parse token, reason: %s", err)
	}

	var expiration time.Time
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		// Check if the token has exp claim
		if claims["exp"] == nil {
			return false, fmt.Errorf("could not parse token, reason: no expiration date")
		}
		switch iat := claims["exp"].(type) {
		case float64:
			expiration = time.Unix(int64(iat), 0)
		case json.Number:
			tmp, _ := iat.Int64()
			expiration = time.Unix(tmp, 0)
		}
	} else {
		return false, fmt.Errorf("broken token (claims are empty): %v\nerror: %s", claims, err)
	}

	tomorrow := time.Now().AddDate(0, 0, 1)

	return tomorrow.After(expiration), nil
}

func ListFiles(config Config, prefix string) (result *s3.ListObjectsV2Output, err error) {
	sess := session.Must(session.NewSession(&aws.Config{
		// The region for the backend is always the specified one
		// and not present in the configuration from auth - hardcoded
		Region:           aws.String("us-west-2"),
		Credentials:      credentials.NewStaticCredentials(config.AccessKey, config.AccessKey, config.AccessToken),
		Endpoint:         aws.String(config.HostBase),
		DisableSSL:       aws.Bool(!config.UseHTTPS),
		S3ForcePathStyle: aws.Bool(true),
	}))

	svc := s3.New(sess)

	result, err = svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(config.AccessKey + "/"),
		Prefix: aws.String(config.AccessKey + "/" + prefix),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list objects, reason: %v", err)
	}

	return result, nil
}
