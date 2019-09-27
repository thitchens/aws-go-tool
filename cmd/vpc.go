package cmd

import (
	"fmt"

	"github.com/afeeblechild/aws-go-tool/lib/utils"
	"github.com/afeeblechild/aws-go-tool/lib/vpc"
	"github.com/spf13/cobra"
)

var vpcCmd = &cobra.Command{
	Use:   "vpc",
	Short: "For use with interacting with the vpc service",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Run -h to see the help menu")
	},
}

var subnetsListCmd = &cobra.Command{
	Use:   "subnetslist",
	Short: "Will generate a report of vpc info for all given accounts",
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := utils.BuildAccountsSlice(ProfilesFile, AccessType)
		if err != nil {
			fmt.Println(err)
			return
		}

		profilesSubnets, err := vpc.GetProfilesSubnets(accounts)
		if err != nil {
			fmt.Println(err)
			return
		}
		//var tags []string
		//if TagFile != "" {
		//	tags, err = utils.ReadFile(TagFile)
		//	if err != nil {
		//		log.Println("could not open tagFile:", err, "\ncontinuing without tags in output")
		//		fmt.Println("could not open tagFile:", err)
		//		fmt.Println("continuing without tags in output")
		//	}
		//}
		//options := utils.Ec2Options{Tags:tags}
		err = vpc.WriteProfilesSubnets(profilesSubnets)
		if err != nil {
			fmt.Println(err)
			return
		}
	},
}

var vpcsListCmd = &cobra.Command{
	Use:   "vpcslist",
	Short: "Will generate a report of vpc info for all given accounts",
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := utils.BuildAccountsSlice(ProfilesFile, AccessType)
		if err != nil {
			fmt.Println(err)
			return
		}

		profilesVpcs, err := vpc.GetProfilesVpcs(accounts)
		if err != nil {
			fmt.Println(err)
			return
		}
		//var tags []string
		//if TagFile != "" {
		//	tags, err = utils.ReadFile(TagFile)
		//	if err != nil {
		//		log.Println("could not open tagFile:", err, "\ncontinuing without tags in output")
		//		fmt.Println("could not open tagFile:", err)
		//		fmt.Println("continuing without tags in output")
		//	}
		//}
		//options := utils.Ec2Options{Tags:tags}
		err = vpc.WriteProfilesVpcs(profilesVpcs)
		if err != nil {
			fmt.Println(err)
			return
		}
	},
}

func init() {
	RootCmd.AddCommand(vpcCmd)

	vpcCmd.AddCommand(vpcsListCmd)
	vpcCmd.AddCommand(subnetsListCmd)
}
