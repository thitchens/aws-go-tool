package ec2

import (
	"encoding/csv"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/afeeblechild/aws-go-tool/lib/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type RegionSnapshots struct {
	Profile   string
	AccountId string
	Region    string
	Snapshots []ec2.Snapshot
	//The Volumes info here will be used to determine the attachment status of volumes on the snapshots
	Volumes []ec2.Volume
}

type AccountSnapshots []RegionSnapshots
type ProfilesSnapshots []AccountSnapshots

// GetRegionSnapshots will take a session and get all snapshots based on the region of the session
func (rs *RegionSnapshots) GetRegionSnapshots(sess *session.Session, accountId string) error {
	svc := ec2.New(sess)
	params := &ec2.DescribeSnapshotsInput{
		OwnerIds: aws.StringSlice([]string{accountId}),
	}

	for {
		resp, err := svc.DescribeSnapshots(params)
		if err != nil {
			return err
		}
		for _, snapshot := range resp.Snapshots {
			rs.Snapshots = append(rs.Snapshots, *snapshot)
		}

		if resp.NextToken != nil {
			params.NextToken = resp.NextToken
		} else {
			break
		}
	}

	return nil
}

// GetAccountSnapshots will take a profile and go through all regions to get all snapshots in the account
func GetAccountSnapshots(account utils.AccountInfo) (AccountSnapshots, error) {
	profile := account.Profile
	fmt.Println("Getting snapshots for profile:", profile)
	snapshotsChan := make(chan RegionSnapshots)
	var wg sync.WaitGroup

	for _, region := range utils.RegionMap {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()
			info := RegionSnapshots{AccountId: account.AccountId, Region: region, Profile: profile}
			sess, err := account.GetSession(region)
			if err != nil {
				log.Println("Could not get snapshots for", account.Profile, ":", err)
				return
			}
			if err = info.GetRegionSnapshots(sess, info.AccountId); err != nil {
				log.Println("Could not get snapshots for", account.Profile, ":", err)
				return
			}
			info.Volumes, err = GetRegionVolumes(sess)
			if err != nil {
				log.Println("Could not get volumes for", account.Profile, ":", err)
			}
			snapshotsChan <- info
		}(region)
	}

	go func() {
		wg.Wait()
		close(snapshotsChan)
	}()

	var accountSnapshots AccountSnapshots
	for regionSnapshots := range snapshotsChan {
		accountSnapshots = append(accountSnapshots, regionSnapshots)
	}

	return accountSnapshots, nil
}

// GetProfilesSnapshots will return all the snapshots in all accounts of a given filename with a list of profiles in it
func GetProfilesSnapshots(accounts []utils.AccountInfo) (ProfilesSnapshots, error) {
	profilesSnapshotsChan := make(chan AccountSnapshots)
	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		go func(account utils.AccountInfo) {
			defer wg.Done()
			if err := account.SetAccountId(); err != nil {
				log.Println("could not set account id for", account.Profile, ":", err)
				return
			}
			accountSnapshots, err := GetAccountSnapshots(account)
			if err != nil {
				return
			}
			profilesSnapshotsChan <- accountSnapshots
		}(account)
	}

	go func() {
		wg.Wait()
		close(profilesSnapshotsChan)
	}()

	var profilesSnapshots ProfilesSnapshots
	for accountSnapshots := range profilesSnapshotsChan {
		profilesSnapshots = append(profilesSnapshots, accountSnapshots)
	}
	return profilesSnapshots, nil
}

func WriteProfilesSnapshots(profileSnapshots ProfilesSnapshots, options utils.Ec2Options) error {
	outputDir := "output/ec2/"
	utils.MakeDir(outputDir)
	outputFile := outputDir + "snapshots.csv"
	outfile, err := utils.CreateFile(outputFile)
	if err != nil {
		return fmt.Errorf("could not create snapshots file: %v", err)
	}
	writer := csv.NewWriter(outfile)
	defer writer.Flush()
	fmt.Println("Writing snapshots to file:", outfile.Name())
	var columnTitles = []string{"Profile",
		"Account ID",
		"Region",
		"Snapshot Name",
		"Snapshot ID",
		"Volume ID",
		"Associated Instance",
		"Size",
		"Status",
		"Start Date",
		"Encrypted",
		"KMS Key ID",
		"Snapshot Description",
	}
	tags := options.Tags
	if len(tags) > 0 {
		for _, tag := range tags {
			columnTitles = append(columnTitles, tag)
		}
	}

	err = writer.Write(columnTitles)

	for _, accountSnapshots := range profileSnapshots {
		for _, regionSnapshots := range accountSnapshots {
			for _, snapshot := range regionSnapshots.Snapshots {
				var snapshotName string
				for _, tag := range snapshot.Tags {
					if *tag.Key == "Name" {
						snapshotName = *tag.Value
					}
				}
				//need to get kmsID separately as not all snapshots will have one and trying to print *snapshot.KmsKeyId directly will fail if it doesn't have one
				var kmsID string
				if snapshot.KmsKeyId != nil {
					kmsID = *snapshot.KmsKeyId
				} else {
					kmsID = "nil"
				}

				var volumeAttachment string
				volumeAttachment = "N/A"
				if *snapshot.VolumeId != "vol-ffffffff" {
					for _, volume := range regionSnapshots.Volumes {
						if *volume.VolumeId == *snapshot.VolumeId {
							for _, attachment := range volume.Attachments {
								if *attachment.State == "attached" {
									volumeAttachment = *attachment.InstanceId
								} else {
									volumeAttachment = "unattached"
								}
							}
						}
					}
				}
				if volumeAttachment == "N/A" && strings.Contains(*snapshot.Description, "CreateImage") {
					//regex search to find the instance id, but it does grab more than instance ids, so there is another check later to filter this
					r, err := regexp.Compile("(?i)\\b[a-z]+-[a-z0-9]+")
					if err != nil {
						fmt.Println(err)
					}

					exp := r.FindString(*snapshot.Description)
					//if the regex string does not contain "ami" or "snap" or "vol, then it is an instance id
					//this is to identify instance ids from the "CreateImage" description on ami snapshots
					if !strings.Contains(exp, "ami") && !strings.Contains(exp, "snap") && !strings.Contains(exp, "vol") {
						volumeAttachment = exp
					}
				}

				splitDate := strings.Split(snapshot.StartTime.String(), " ")
				startDate := splitDate[0]

				var data = []string{regionSnapshots.Profile,
					regionSnapshots.AccountId,
					regionSnapshots.Region,
					snapshotName,
					*snapshot.SnapshotId,
					*snapshot.VolumeId,
					volumeAttachment,
					strconv.Itoa(int(*snapshot.VolumeSize)),
					*snapshot.State,
					startDate,
					strconv.FormatBool(*snapshot.Encrypted),
					kmsID,
					*snapshot.Description,
				}

				if len(tags) > 0 {
					for _, tag := range tags {
						x := false
						for _, snapshotTag := range snapshot.Tags {
							if *snapshotTag.Key == tag {
								data = append(data, *snapshotTag.Value)
								x = true
							}
						}
						if !x {
							data = append(data, "")
						}
					}
				}

				err = writer.Write(data)
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	}
	return nil
}
