package main

// Copyright (c) 2009 & onwards. MapR Tech, Inc., All rights reserved

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	install_dir         = "/opt/mapr"
	copy_path           = "/etc/kubernetes/mapr-kdf"
	k8s_dir             = install_dir + "/k8s"
	log_path            = install_dir + "/logs"
	client_lib_path     = install_dir + "/lib"
	client_bin_path     = k8s_dir + "/bin"
	client_mounts_path  = k8s_dir + "/mounts/"
	client_support_path = k8s_dir + "/support/"
	save_file           = k8s_dir + "/SAVE"
	info_file           = k8s_dir + "/serviceinfo"
	plugin_log          = log_path + "/plugin-k8s.log"
	fuse_script         = client_bin_path + "/start-fuse"
	copy_script         = copy_path + "/copy2mapr"
	cldb_default_port   = ":7222"
	ticket_key          = "CONTAINER_TICKET"
)

var (
	Plugin     *log.Logger
	master     = flag.String("master", "", "Master URL")
	kubeconfig = flag.String("kubeconfig", "", "Absolute path to the kubeconfig")
)

// usage displays supported methods for plugin
func usage() {
	fmt.Print("Invalid usage. Usage: ")
	fmt.Print("\t$0 init")
	fmt.Print("\t$0 mount <mount dir> <json params>")
	fmt.Print("\t$0 unmount <mount dir>")
	os.Exit(1)
}

// cleanup old installations of the plugin
func cleanup() {
	Plugin.Println("INFO  Cleaning up old mounts")
	_, err := os.Stat(client_support_path)
	if err == nil {
		files, err := ioutil.ReadDir(client_support_path)
		if err != nil {
			Plugin.Printf("ERROR  Error scanning support dir. Reason: %v", err)
			return
		}
		for _, file := range files {
			path := client_support_path + file.Name()
			Plugin.Printf("INFO  Cleaning up %s", path)
			kpathfile := path + "/kpath"
			kpathinfo, err := ioutil.ReadFile(kpathfile)
			if err != nil {
				Plugin.Printf("ERROR  Failed to read kpath: %s Reason: %v", kpathfile, err)
				Plugin.Printf("ERROR  Cannot cleanup %s", path)
			} else {
				kpath := string(kpathinfo)
				args := []string{"1", "2", strings.Trim(kpath, "\n"), "4"}
				Plugin.Printf("INFO  Calling unmount for %s", args[2])
				unmount(args, true)
			}

		}
		Plugin.Println("INFO  Finished cleaning old mounts")
	} else {
		Plugin.Println("INFO  No old mounts to remove")
	}
}

// linkFiles sets lib linking on plugin init
func linkFiles() {
	Plugin.Println("INFO  Linking fusermount")
	to := "/bin/fusermount"
	from := client_bin_path + "/fusermount"
	err := os.Symlink(from, to)
	if err != nil {
		Plugin.Printf("INFO  Will not create link from %s to %s. Reason: %v ", to, from, err)
	} else {
		Plugin.Println("INFO  Created link from %s to %s.", to, from)
	}
}

// get podid from kpath string
func getPodId(kpath string) string {
	podslice := strings.Split(kpath, "/")
	slicelen := len(podslice)
	idx := slicelen - 4
	podid := podslice[idx]
	return podid
}

// get podid from kpath string
func getVolId(kpath string) string {
	volslice := strings.Split(kpath, "/")
	slicelen := len(volslice)
	idx := slicelen - 1
	volid := volslice[idx]
	return volid
}

// startFuse mounts a FUSE filesystem at fmount
func startFuse(kpath string, options map[string]string) string {
	// set variables
	/*
	   unused params include:
	     kubernetes.io/fsType, kubernetes.io/pod.name,
	     kubernetes.io/pod.namespace, kubernetes.io/readwrite,
	     kubernetes.io/serviceAccount.name, kubernetes.io/pvOrVolumeName
	*/
	readoptions := options["kubernetes.io/readwrite"]
	readOnly := options["readOnly"]
	cluster := options["cluster"]
	cldb := options["cldbHosts"]
	sectype := options["securityType"]
	platinum := options["platinum"]
	mountOptions := options["mountOptions"]
	tsname := options["ticketSecretName"]
	tsnamespace := options["ticketSecretNamespace"]
	ticket := ""
	ticketname := "maprticket_0"
	args := "-o allow_other -o big_writes -o auto_unmount -o async_dio -o max_background=64 -o auto_inval_data --disable_writeback"
	podid := getPodId(kpath)
	volid := getVolId(kpath)
	fmount := client_mounts_path + podid + "-" + volid
	spath := client_support_path + podid + "-" + volid
	ticketfile := spath + "/" + ticketname
	ffspath := log_path + "/" + podid + "-" + volid
	var sDec []byte
	Plugin.Println("INFO  Starting FUSE")
	if strings.TrimSpace(mountOptions) != "" {
		Plugin.Printf("INFO  Mounting MapRfs with options: %s", mountOptions)
		args = mountOptions
	}
	Plugin.Printf("INFO  Kubernetes readoptions: %s readOnly: %s", readoptions, readOnly)
	if readoptions != "rw" || readOnly == "true" {
		Plugin.Printf("INFO  Mounting MapRfs as Read-Only")
		args = args + " -o ro"
	}
	// make sure we have enough info to mount FUSE
	if (cluster == "") || (cldb == "") {
		Plugin.Println("ERROR  Cluster or CldbHosts is blank")
		fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Must specify a Cluster and CldbHosts \" }")
		os.Exit(1)
	}
	// create FUSE Mount directory
	Plugin.Printf("INFO  Create FUSE mount directory %s...", fmount)
	err := os.MkdirAll(fmount, 0700)
	if err != nil {
		Plugin.Printf("ERROR  Failed to create FUSE mount directory: %s Reason: %v", fmount, err)
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to create FUSE mount directory: %s \" }", fmount)
		os.Exit(1)
	}
	// create FUSE Support directory
	Plugin.Printf("INFO  Creating FUSE support directory at: %s ...", spath)
	err = os.MkdirAll(spath, 0700)
	if err != nil {
		Plugin.Printf("ERROR  Failed to create FUSE support directory: %s Reason: %v", spath, err)
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to create FUSE support directory: %s \" }", spath)
		os.Exit(1)
	}
	// Fix CLDB Server Ports
	cldblist := ""
	cldbservers := strings.Split(cldb, " ")
	for idx := range cldbservers {
		cldbserver := cldbservers[idx]
		if !strings.Contains(cldbserver, ":") {
			cldbserver = cldbserver + cldb_default_port
		}
		cldblist = cldblist + cldbserver + " "
	}
	// set cluster conf line
	conf := cluster + " secure=false " + cldblist
	// if secure, write out ticket
	if sectype == "secure" {
		// check to make sure ticket is available if we have a secure cluster
		Plugin.Printf("INFO  ticketSecretName=%s ticketSecretName=%s", tsname, tsnamespace)
		if tsname == "" || tsnamespace == "" {
			Plugin.Println("ERROR  Must pass a MapR Ticket or secret if specifying secure")
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Must pass a MapR Ticket if you specify secure \" }")
			os.Exit(1)
		}
		// read INFO
		info, err := ioutil.ReadFile(info_file)
		if err != nil {
			Plugin.Printf("ERROR  Failed to read INFO file. Reason: %s", err)
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Failed to read INFO file \" }")
			os.Exit(1)
		} else {
			sinfo := string(info)
			sinfo = strings.Trim(sinfo, "\n")
			kinfo := strings.Split(sinfo, ":")
			Plugin.Printf("INFO  Setting khost=%s kport=%s", kinfo[0], kinfo[1])
			os.Setenv("KUBERNETES_SERVICE_HOST", kinfo[0])
			os.Setenv("KUBERNETES_SERVICE_PORT", kinfo[1])
			os.Setenv("KUBERNETES_SERVICE_PORT_HTTPS", kinfo[1])
		}
		// Call kubeclient to get ticket in secret
		Plugin.Println("INFO  Starting kube client...")
		var config *rest.Config
		config, err = rest.InClusterConfig()
		if err != nil {
			Plugin.Println("ERROR  Failed to create config to get ticket: %v", err)
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Failed to create config to get ticket \" }")
			os.Exit(1)
		}
		// Create client
		kubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			Plugin.Printf("ERROR  Failed to create client to get ticket: %v", err)
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Failed to create client to get ticket \" }")
			os.Exit(1)
		}
		secretMap := make(map[string]string)
		if kubeClient == nil {
			Plugin.Println("ERROR  Cannot get kube client")
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Cannot get kube client \" }")
			os.Exit(1)
		}
		// Get Secret Data
		secrets, err := kubeClient.Core().Secrets(tsnamespace).Get(tsname, metav1.GetOptions{})
		if err != nil {
			Plugin.Printf("ERROR  Can't get secrets. Error: %v", err)
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Can't get secrets \" }")
			os.Exit(1)
		}
		// Extract Ticket
		for name, data := range secrets.Data {
			secretMap[name] = string(data)
		}
		if len(secretMap) == 0 {
			Plugin.Printf("ERROR  Empty secret map data")
			fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Empty secret map data \" }")
			os.Exit(1)
		}
		for k, v := range secretMap {
			if k == ticket_key {
				ticket = v
				Plugin.Println("INFO  secret=%s", ticket)
				Plugin.Println("INFO  Got ticket secret")
			}
		}
		// Write Ticket
		sDec = []byte(ticket)
		Plugin.Printf("INFO  Creating ticket: %s for secure mount ...", ticketfile)
		err = ioutil.WriteFile(ticketfile, sDec, 0600)
		if err != nil {
			Plugin.Printf("ERROR  Failed to write ticket: %s Reason: %v", ticketfile, err)
			fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to write ticket: %s  \" }", ticketfile)
			os.Exit(1)
		}
		conf = cluster + " secure=true " + cldblist
		Plugin.Printf("INFO  Creating cluster info: %s", conf)
	}
	// Setup command to call FUSE
	Plugin.Printf("INFO  Calling FUSE script: %s...", fuse_script)
	cmd := exec.Command(fuse_script, fmount, spath, ffspath, kpath, sectype, ticketfile, conf, platinum, podid, args)
	// Call FUSE
	err = cmd.Run()
	if err != nil {
		Plugin.Printf("ERROR  Failed to mount: %s Reason: %v", kpath, err)
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to mount: %s \" }", kpath)
		os.Exit(1)
	}
	Plugin.Println("INFO  Successfully started FUSE")
	return fmount
}

// stop FUSE located at fmount
func stopFuse(fmount string) error {
	Plugin.Printf("INFO  Stopping FUSE mount at: %s", fmount)
	// unmount FUSE
	_, err := os.Stat(fmount)
	if err == nil {
		if err := syscall.Unmount(fmount, syscall.MNT_FORCE); err != nil {
			Plugin.Printf("ERROR  Failed to unmount: %s Reason: %v", fmount, err)
			return err
		} else {
			Plugin.Printf("INFO  Unmounted FUSE mount at: %s", fmount)
		}
		if err := os.Remove(fmount); err != nil {
			Plugin.Printf("ERROR  Failed to remove directory: %s Reason: %v", fmount, err)
			return err
		} else {
			Plugin.Printf("INFO  Removed FUSE directory at: %s", fmount)
		}

	} else {
		Plugin.Println("INFO  FUSE directory doesnt exist")
	}
	return nil
}

// doMount mounts MapRFS to a kubernetes supplied directory
func doMount(args []string) {
	kpath := args[2]
	voptions := args[3]
	var options map[string]string
	json.Unmarshal([]byte(voptions), &options)
	vpath := options["volumePath"]
	cluster := options["cluster"]
	Plugin.Printf("INFO  === Starting new mount at %s ===", kpath)
	if kpath == "" {
		Plugin.Println("ERROR  Kubernetes did not pass the Kubernetes Path. Cannot mount without Kubernetes Path")
		fmt.Print("{ \"status\": \"Failure\" , \"message\": \"Kubernetes did not pass the Kubernetes Path. Cannot mount without Kubernetes Path\" }")
		os.Exit(1)
	}
	if vpath == "" {
		Plugin.Println("WARNING  Should specify a volumePath. Using / ")
		vpath = "/"
	}
	// create FUSE mount
	fpath := startFuse(kpath, options)
	mpath := fpath + "/" + cluster + vpath
	Plugin.Printf("INFO  Creating bind mount directory %s...", mpath)
	err := os.MkdirAll(mpath, 0700)
	if err != nil {
		Plugin.Printf("ERROR  Failed to create bind mount directory: %s Reason: %v", mpath, err)
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to create bind mount directory: %s \" }", mpath)
		os.Exit(1)
	}
	// create bind mount
	Plugin.Printf("INFO  Mounting %s at: %s", mpath, kpath)
	cmd := exec.Command("mount", "--bind", mpath, kpath)
	err = cmd.Run()
	if err != nil {
		Plugin.Printf("ERROR  Failed to mount: %s at: %s Reason: %v", mpath, kpath, err)
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to mount: %s at: %s \" }", kpath, mpath)
		os.Exit(1)
	}
	Plugin.Printf("INFO  === Successfully mounted: %s ===", kpath)
	fmt.Print("{ \"status\": \"Success\" }")
	os.Exit(0)
}

// unmounts previously created FUSE mount
func unmount(args []string, cleanup bool) {
	error := false
	kpath := args[2]
	Plugin.Printf("INFO  === Starting unmount of %s ===", kpath)
	podid := getPodId(kpath)
	volid := getVolId(kpath)
	fmount := client_mounts_path + podid + "-" + volid
	spath := client_support_path + podid + "-" + volid

	// do bind unmount
	Plugin.Println("INFO  Unmount bind mount...")
	_, err := os.Stat(kpath)
	if err == nil {
		if err := syscall.Unmount(kpath, syscall.MNT_FORCE); err != nil {
			Plugin.Printf("ERROR  Failed to unmount: %s Reason: %v", fmount, err)
			error = true
		} else {
			Plugin.Printf("INFO  Unmounted BIND mount at: %s", fmount)
		}
		if err := os.RemoveAll(kpath); err != nil {
			Plugin.Printf("ERROR  Failed to remove directory: %s Reason: %v", fmount, err)
			error = true
		} else {
			Plugin.Printf("INFO  Removed BIND directory at: %s", fmount)
		}
	}
	// do fuse unmount
	err = stopFuse(fmount)
	if err != nil {
		error = true
	}
	// read fusepid
	fpidfile := spath + "/fusepid"
	fpid, err := ioutil.ReadFile(fpidfile)
	if err != nil {
		Plugin.Printf("ERROR  Failed to read FUSE pid: %s Reason: %v", fpidfile, err)
		error = true
	} else {
		spid := string(fpid)
		ipid, err := strconv.Atoi(strings.Trim(spid, "\n"))
		if err != nil {
			Plugin.Printf("ERROR  Failed to convert FUSE pid: %s to int. Reason: %v", fpid, err)
			error = true
		}
		// kill fuse
		syscall.Kill(ipid, syscall.SIGKILL)
		if err != nil {
			Plugin.Printf("ERROR  Failed to kill FUSE process pid: %s Reason: %v", ipid, err)
			error = true
		} else {
			Plugin.Printf("INFO  Killed FUSE process pid: %d", ipid)
		}
	}

	// Remove support dir
	_, err = os.Stat(spath)
	if err == nil {
		if err := os.RemoveAll(spath); err != nil {
			Plugin.Printf("ERROR  Failed to remove directory: %s Reason: %v", spath, err)
			error = true
		} else {
			Plugin.Printf("INFO  Removed support directory: %s", spath)
		}
	}
	if error == true && cleanup == false {
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"ERROR Failed to unmount: %s \" }", kpath)
		os.Exit(1)
	}
	Plugin.Printf("INFO  === Successfully unmounted %s ===", kpath)
	if cleanup == false {
		fmt.Print("{ \"status\": \"Success\" }")
		os.Exit(0)
	}
}

// doInit prepares host node for MapRFS plugin
func doInit() {
	Plugin.Println("INFO  === Starting init of MapRfs Plugin ===")
	cmd := exec.Command(copy_script)
	err := cmd.Run()
	if err != nil {
		Plugin.Printf("ERROR  Failed to do 2nd stage copy. Reason: %v", err)
		fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to do 2nd stage copy. \" }")
		os.Exit(1)
	}
	_, err = os.Stat(save_file)
	if err != nil {
		cleanup()
		linkFiles()
		// set hostname for FUSE
		Plugin.Printf("INFO  Setting hostname... %s/hostname", k8s_dir)
		cmd = exec.Command("/bin/hostname", "--fqdn")
		out, err := cmd.Output()
		hostFile := k8s_dir + "/hostname"
		err = ioutil.WriteFile(hostFile, out, 0600)
		if err != nil {
			Plugin.Printf("ERROR  Failed to create: %s Reason: %v", hostFile, err)
			fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to create %s \" }", hostFile)
			os.Exit(1)
		}
		// Set core file pattern
		Plugin.Println("INFO  Setting core pattern...")
		patternFile := "/proc/sys/kernel/core_pattern"
		err = ioutil.WriteFile(patternFile, []byte("/opt/cores/%e.core.%p.%h"), 0600)
		if err != nil {
			Plugin.Printf("ERROR  Failed to create: %s Reason: %s", patternFile, err)
			fmt.Printf("{ \"status\": \"Failure\" , \"message\": \"Failed to create %s \" }", patternFile)
			os.Exit(1)
		}
	} else {
		Plugin.Printf("INFO  Reinstalling same plugin version. Do not clean up to preserve running containers")
		if err := os.Remove(save_file); err != nil {
			Plugin.Printf("ERROR  Failed to remove save file: %s Reason: %v", save_file, err)
		} else {
			Plugin.Printf("INFO  Removed save file: %s", save_file)
		}
	}
	Plugin.Println("INFO  === Finished init of MapRfs Plugin ===")
	fmt.Print(" { \"status\": \"Success\" , \"capabilities\": {\"attach\": false, \"selinuxRelabel\": false } } ")
	os.Exit(0)
}

// main is called on launch. Filters op code and calls relevant method
func main() {
	args := os.Args
	op := os.Args[1]
	_, err := os.Stat(log_path)
	if err != nil {
		os.MkdirAll(log_path, 0700)
	}
	f, err := os.OpenFile(plugin_log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Printf("ERROR  Can't create plugin log! Reason: %v \n", err)
		os.Exit(1)
	}
	defer f.Close()
	Plugin = log.New(f, "", log.Ldate|log.Ltime|log.Lshortfile)
	Plugin.Printf("INFO  mapr.com/maprfs plugin called. Operation: %s", op)
	if op == "init" {
		doInit()
	}
	if len(args) < 2 {
		usage()
	}
	switch op {
	case "mount":
		doMount(args)
	case "unmount":
		unmount(args, false)
	default:
		Plugin.Printf("INFO  Unsupported Plugin Op (%s)", op)
		fmt.Print("{ \"status\": \"Not supported\" }")
		os.Exit(0)
	}
	os.Exit(1)
}
