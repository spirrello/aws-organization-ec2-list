// Main file

package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/spirrello/aws-organization-ec2-list/config"
)

func main() {
	// Retrieve config file
	config := config.InitVariables()

	// Get all accounts names & ID from the organization
	listAccounts := getOrganizationAccounts(config)

	// Create list variable to store every ec2 instances
	var listEc2 = make(map[string][]string)

	// Loop over each account and get its instances via a function
	fmt.Println("Retrieving the instances...")
	for accountName, accountID := range listAccounts {
		listEc2 = getAccountEc2(config, accountName, accountID, listEc2)
	}
	fmt.Println("All the instances from the Organization were retrieved.")

	// Write results to a CSV file
	writeToCSV(listEc2)
}

// Retrieve all accounts within organization
func getOrganizationAccounts(config config.Config) map[string]string {
	// Create organization service client
	var c Clients
	svc := c.Organization(config.Region, config.MasterAccountID, config.OrganizationRole)
	// Create variable for the list of accounts and initialize input
	organizationAccounts := make(map[string]string)
	input := &organizations.ListAccountsInput{}
	// Start a do-while loop
	for {
		// Retrieve the accounts with a limit of 20 per call
		organizationAccountsPaginated, err := svc.ListAccounts(input)
		// Append the accounts from the current call to the total list
		for _, account := range organizationAccountsPaginated.Accounts {
			organizationAccounts[*account.Name] = *account.Id
		}
		checkError("Could not retrieve account list", err)
		// Check if more accounts need to be retrieved using api token, otherwise break the loop
		if organizationAccountsPaginated.NextToken == nil {
			break
		} else {
			input = &organizations.ListAccountsInput{NextToken: organizationAccountsPaginated.NextToken}
		}
	}
	return organizationAccounts
}

// Retrieve all ec2 instances and their attributes within an account
func getAccountEc2(config config.Config, accountName string, accountID string, result map[string][]string) map[string][]string {
	// Create EC2 service client
	var c Clients
	svc := c.EC2(config.Region, accountID, config.OrganizationRole)
	// Get the EC2 list of the given account
	input := &ec2.DescribeInstancesInput{}
	instances, err := svc.DescribeInstances(input)
	checkError("Could not retrieve the EC2s", err)

	// Iterate over the EC2 instances and add elements to global list, if instances > 0
	if len(instances.Reservations) != 0 {
		for _, reservation := range instances.Reservations {
			// Loop through every individual EC2 instance
			for _, instance := range reservation.Instances {
				// Set the map key using the unique instance ID
				key := *instance.InstanceId
				// Retrieve account information
				result[key] = append(result[key], accountName)
				result[key] = append(result[key], accountID)
				// Check if the instance name is set using tags, otherwise use default null name
				for _, tag := range instance.Tags {
					if *tag.Key == "Name" {
						result[key] = append(result[key], *tag.Value)

					}
				}
				if len(result) == 2 {
					result[key] = append(result[key], "N/A")
				}
				// Retrieve instance information, some use default values  if potentially null
				result[key] = append(result[key], *instance.InstanceType)
				result[key] = append(result[key], *instance.InstanceId)
				result[key] = append(result[key], *instance.ImageId)
				if instance.Platform != nil {
					result[key] = append(result[key], *instance.Platform)
				} else {
					result[key] = append(result[key], "linux")
				}
				if instance.PrivateIpAddress != nil {
					result[key] = append(result[key], *instance.PrivateIpAddress)
				} else {
					result[key] = append(result[key], "N/A")
				}
				result[key] = append(result[key], *instance.State.Name)
				result[key] = append(result[key], (*instance.LaunchTime).String())
			}
		}
	}
	fmt.Println("Account number " + accountID + " done")
	return result
}

// Clients Struct to store the session with custom parameters
type Clients struct {
	session *session.Session
	configs map[string]*aws.Config
}

// Session Func to start a session
func (c Clients) Session() *session.Session {
	if c.session != nil {
		return c.session
	}
	sess := session.Must(session.NewSession())
	c.session = sess
	return sess
}

// Config custom func
func (c Clients) Config(
	region *string,
	accountID *string,
	role *string) *aws.Config {

	// return no config for nil inputs
	if accountID == nil || region == nil || role == nil {
		return nil
	}
	arn := fmt.Sprintf(
		"arn:aws:iam::%v:role/%v",
		*accountID,
		*role,
	)
	// include region in cache key otherwise concurrency errors
	key := fmt.Sprintf("%v::%v", *region, arn)

	// check for cached config
	if c.configs != nil && c.configs[key] != nil {
		return c.configs[key]
	}
	// new creds
	creds := stscreds.NewCredentials(c.Session(), arn)
	// new config
	config := aws.NewConfig().
		WithCredentials(creds).
		WithRegion(*region).
		WithMaxRetries(10)
	if c.configs == nil {
		c.configs = map[string]*aws.Config{}
	}
	c.configs[key] = config
	return config
}

// Organization Create client
func (c *Clients) Organization(
	region string,
	accountID string,
	role string) *organizations.Organizations {
	return organizations.New(c.Session(), c.Config(&region, &accountID, &role))
}

// EC2 Create client
func (c *Clients) EC2(
	region string,
	accountID string,
	role string) *ec2.EC2 {
	return ec2.New(c.Session(), c.Config(&region, &accountID, &role))
}

// Function that log errors if not null
func checkError(message string, err error) {
	if err != nil {
		log.Fatal(message, err)
	}
}

// Function that writes a map of slices to a CSV File
func writeToCSV(listEc2 map[string][]string) {
	// Create the csv file using the os package
	fmt.Println("Creating a CSV file...")
	file, err := os.Create("result.csv")
	checkError("Cannot create file", err)
	defer file.Close()
	// Create the writer object
	writer := csv.NewWriter(file)
	defer writer.Flush()
	// Write headers
	var headers = []string{"Account Name", "Account ID", "Instance Name", "Instance Size", "Instance ID", "Image ID", "Platform", "Private IP", "State", "Timestamp"}
	writer.Write(headers)
	// Loop over the organization ec2 list and write them in rows in the csv file
	for _, value := range listEc2 {
		err := writer.Write(value)
		checkError("Cannot write to file", err)
	}
	fmt.Println("CSV file created in " + "result.csv")
}
