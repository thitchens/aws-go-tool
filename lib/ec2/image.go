package ec2

import (
	"encoding/csv"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/afeeblechild/aws-go-tool/lib/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type (
	ImageInfo struct {
		Image     ec2.Image
		InUse     bool //if the AMI is in use by any current instance
		Count     int  //how many instances use the AMI
		AccountId string
		Profile   string
		Region    string
	}

	AmiOptions struct {
		Tags []string
	}

	RegionImages struct {
		Profile   string
		AccountId string
		Region    string
		Images    []ec2.Image
	}

	AccountImages  []RegionImages
	ProfilesImages []AccountImages
)

// GetRegionImages will take a session and pull all amis based on the region of the session
func (ri *RegionImages) GetRegionImages(sess *session.Session, accountId string) error {
	svc := ec2.New(sess)
	params := &ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{accountId}),
	}

	for {
		resp, err := svc.DescribeImages(params)
		if err != nil {
			return err
		}

		for _, image := range resp.Images {
			ri.Images = append(ri.Images, *image)
		}

		if resp.NextToken != nil {
			params.NextToken = resp.NextToken
		} else {
			break
		}
	}

	return nil
}

// GetAccountImages will take a profile and go through all regions to get all amis in the account
func GetAccountImages(account utils.AccountInfo) (AccountImages, error) {
	profile := account.Profile
	fmt.Println("Getting images for profile:", profile)
	imagesChan := make(chan RegionImages)
	var wg sync.WaitGroup

	for _, region := range utils.RegionMap {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()
			info := &RegionImages{AccountId: account.AccountId, Region: region, Profile: profile}
			sess, err := account.GetSession(region)
			if err != nil {
				log.Println("Could not get session for", account.Profile, ":", err)
				return
			}
			if err = info.GetRegionImages(sess, account.AccountId); err != nil {
				log.Println("Could not get images for", region, "in", profile, ":", err)
				return
			}
			imagesChan <- *info
		}(region)
	}

	go func() {
		wg.Wait()
		close(imagesChan)
	}()

	var accountImages AccountImages
	for regionImages := range imagesChan {
		accountImages = append(accountImages, regionImages)
	}

	return accountImages, nil
}

// GetProfilesImages will return all the images in all accounts of a given filename with a list of profiles in it
func GetProfilesImages(accounts []utils.AccountInfo) (ProfilesImages, error) {
	profilesImagesChan := make(chan AccountImages)
	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		go func(account utils.AccountInfo) {
			defer wg.Done()
			if err := account.SetAccountId(); err != nil {
				log.Println("could not set account id for", account.Profile, ":", err)
				return
			}
			accountImages, err := GetAccountImages(account)
			if err != nil {
				log.Println("Could not get images for", account.Profile, ":", err)
				return
			}
			profilesImagesChan <- accountImages
		}(account)
	}

	go func() {
		wg.Wait()
		close(profilesImagesChan)
	}()

	var profilesImages ProfilesImages
	for accountImages := range profilesImagesChan {
		profilesImages = append(profilesImages, accountImages)
	}
	return profilesImages, nil
}

func WriteProfilesImages(profileImages ProfilesImages, options utils.Ec2Options) error {
	outputDir := "output/ec2/"
	utils.MakeDir(outputDir)
	outputFile := outputDir + "images.csv"
	outfile, err := utils.CreateFile(outputFile)
	if err != nil {
		return fmt.Errorf("could not create images file: %v", err)
	}

	fmt.Println("Writing images to file:", outfile.Name())
	writer := csv.NewWriter(outfile)
	defer writer.Flush()

	var columnTitles = []string{"Profile",
		"Account ID",
		"Region",
		"Image Name",
		"Description",
		"Image ID",
		"Creation Time",
	}

	tags := options.Tags
	if len(tags) > 0 {
		for _, tag := range tags {
			columnTitles = append(columnTitles, tag)
		}
	}

	if err = writer.Write(columnTitles); err != nil {
		fmt.Println(err)
	}
	for _, accountImages := range profileImages {
		for _, regionImages := range accountImages {
			for _, image := range regionImages.Images {
				var imageName string
				for _, tag := range image.Tags {
					if *tag.Key == "Name" {
						imageName = *tag.Value
					}
				}

				var description string
				if image.Description != nil {
					description = *image.Description
				}

				splitDate := strings.Split(*image.CreationDate, "T")
				startDate := splitDate[0]

				var data = []string{regionImages.Profile,
					regionImages.AccountId,
					regionImages.Region,
					imageName,
					description,
					*image.ImageId,
					startDate,
				}

				if len(tags) > 0 {
					for _, tag := range tags {
						x := false
						for _, imageTag := range image.Tags {
							if *imageTag.Key == tag {
								data = append(data, *imageTag.Value)
								x = true
							}
						}
						if !x {
							data = append(data, "")
						}
					}
				}

				if err = writer.Write(data); err != nil {
					fmt.Println(err)
				}
			}
		}
	}
	return nil
}

func CheckImages(accounts []utils.AccountInfo) ([]ImageInfo, error) {
	profilesImages, err := GetProfilesImages(accounts)
	if err != nil {
		return nil, fmt.Errorf("could not get profiles images: %v", err)
	}
	profilesInstances, err := GetProfilesInstances(accounts)
	if err != nil {
		return nil, fmt.Errorf("could not get profiles instances: %v", err)
	}

	var checkedImages []ImageInfo
	//loop through images
	for _, accountImages := range profilesImages {
		for _, regionImages := range accountImages {
			for _, image := range regionImages.Images {
				profile := regionImages.Profile
				account := regionImages.AccountId
				region := regionImages.Region
				imageId := *image.ImageId
				var instances []ec2.Instance

				//get the instances for the same account/region as image
				for _, accountInstances := range profilesInstances {
					for _, regionInstances := range accountInstances {
						if account == regionInstances.AccountId && region == regionInstances.Region {
							instances = regionInstances.Instances
						}
					}
				}

				//var to keep track if the image is in use or not
				found := false
				count := 0
				//loop through instances to determine if image is in use
				for _, instance := range instances {
					if imageId == *instance.ImageId {
						found = true
						count++
					}
				}
				if found {
					checkedImage := ImageInfo{Image: image, InUse: true, Count: count, Profile: profile, AccountId: account, Region: region}
					checkedImages = append(checkedImages, checkedImage)
				} else if !found {
					checkedImage := ImageInfo{Image: image, InUse: false, Count: 0, Profile: profile, AccountId: account, Region: region}
					checkedImages = append(checkedImages, checkedImage)
				}
			}
		}
	}
	return checkedImages, nil
}

func WriteCheckedImages(checkedImages []ImageInfo, options utils.Ec2Options) error {
	outputDir := "output/ec2/"
	utils.MakeDir(outputDir)
	outputFile := outputDir + "checkedImages.csv"
	outfile, err := utils.CreateFile(outputFile)
	if err != nil {
		return fmt.Errorf("could not create checkedImages file: %v", err)
	}

	writer := csv.NewWriter(outfile)
	defer writer.Flush()
	fmt.Println("Writing images to file:", outfile.Name())

	var columnTitles = []string{"Profile",
		"Account ID",
		"Region",
		"Image Name",
		"Description",
		"InUse",
		"Count",
		"Image ID",
		"Creation Time",
	}

	tags := options.Tags
	if len(tags) > 0 {
		for _, tag := range tags {
			columnTitles = append(columnTitles, tag)
		}
	}

	if err = writer.Write(columnTitles); err != nil {
		fmt.Println(err)
	}
	for _, checkedImage := range checkedImages {
		image := checkedImage.Image
		var imageName string
		for _, tag := range image.Tags {
			if *tag.Key == "Name" {
				imageName = *tag.Value
			}
		}

		var description string
		if image.Description != nil {
			description = *image.Description
		}

		splitDate := strings.Split(*image.CreationDate, "T")
		startDate := splitDate[0]

		var data = []string{checkedImage.Profile,
			checkedImage.AccountId,
			checkedImage.Region,
			imageName,
			description,
			strconv.FormatBool(checkedImage.InUse),
			strconv.Itoa(checkedImage.Count),
			*image.ImageId,
			startDate,
		}

		if len(tags) > 0 {
			for _, tag := range tags {
				x := false
				for _, imageTag := range image.Tags {
					if *imageTag.Key == tag {
						data = append(data, *imageTag.Value)
						x = true
					}
				}
				if !x {
					data = append(data, "")
				}
			}
		}

		if err = writer.Write(data); err != nil {
			fmt.Println(err)
		}
	}
	return nil
}
