package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
)

func optionsFromFile() Options {

	var o Options
	var configFileName string

	/* This is an example config file */
	// {
	//  "UpdateFromWeb":false,
	//  "ScheduleFromWeb":false,
	//  "RedirectUrl":"https://localhost:4430",
	//  "ScheduleUrl":"schedules/2016regular",
	//  "UpdateUrl":"gameTest1.html",
	//  "PwRecoverSecret":"Secret Phrase",
	//	"AdminEmail" : "fred@foo.com",
	//	"AdminEmailPw" : "yabadabadoo"
	// }

	flag.StringVar(&configFileName, "config", "options.json", "configuration file")

	flag.Parse()

	fmt.Println("config file:", configFileName)

	raw, err := ioutil.ReadFile(configFileName)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	err = json.Unmarshal(raw, &o)
	if err != nil {
		fmt.Println("error reading options from", configFileName, ":", err)
		os.Exit(1)
	}

	fmt.Printf("config options are %+v\n", o)

	return o
}
