package main

// Copyright (c) 2009 & onwards. MapR Tech, Inc., All rights reserved

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kubernetes-incubator/external-storage/lib/controller"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"syscall"
)

var (
	master      = flag.String("master", "", "Master URL")
	kubeconfig  = flag.String("kubeconfig", "", "Absolute path to the kubeconfig")
	legalparams = [...]string{"cldbHosts", "maprSecretName", "maprSecretNamespace", "ticketSecretName", "ticketSecretNamespace", "namePrefix", "cluster", "mount", "mountPrefix", "securityType", "restServers", "platinum", "mountOptions", "readOnly", "reclaimPolicy", "advisoryquota", "ae", "aetype", "allowgrant", "allowinherit", "auditenabled", "cluster", "coalesce", "containerallocationfactor", "criticalrereplicationtimeoutsec", "dataauditops", "dbindexlagsecalarmthresh", "dbrepllagsecalarmthresh", "enforceminreplicationforio", "forceauditenable", "group", "inherit", "localvolumehost", "localvolumeport", "maxinodesalarmthreshold", "maxnssizembalarmthreshold", "minreplication", "mirrorschedule", "mirrorthrottle", "nsminreplication", "nsreplication", "readAce", "readonly", "replication", "replicationtype", "rereplicationtimeoutsec", "rootdirperms", "schedule", "skipinherit", "source", "tenantuser", "topology", "type", "user", "wiresecurityenabled", "writeaAce"}
	Plog        *log.Logger
)

const (
	provision_log      = "/opt/mapr/logs/provisioner-k8s.log"
	provisionerName    = "mapr.com/maprfs"
	driverName         = "mapr.com/maprfs"
	provisionerVersion = "v1.0.0"
	randomSize         = 10
	adminUserKey       = "MAPR_CLUSTER_USER"
	adminPasswordKey   = "MAPR_CLUSTER_PASSWORD"
	adminMaprTicketKey = "MAPR_TICKET"
	restServerPort     = ":8443"
)

// maprProvisioner is the struct that is used to provision MapR volumes
type maprProvisioner struct {
	kubeClient kubernetes.Interface
	// Identity of this maprProvisioner, set to node's name. Used to identify
	// "this" provisioner's PVs.
	identity      string
	eventRecorder record.EventRecorder
	eventTarget   runtime.Object
}

// volumeInfo is a struct containing info on PV to be created
type volumeInfo struct {
	PVName        string
	Name          string
	Path          string
	Quota         string
	Platinum      string
	ReadOnly      string
	MountOptions  string
	ReclaimPolicy v1.PersistentVolumeReclaimPolicy
}

// serverInfo is a struct containing info on MapR REST server
type serverInfo struct {
	REST                  string
	Cldbhosts             string
	Cluster               string
	Securitytype          string
	Userinfo              url.Userinfo
	RESTSecretName        string
	RESTSecretNamespace   string
	TicketSecretName      string
	TicketSecretNamespace string
}

// randomString generates a random string of specified size
func randomString(l int) string {
	bytes := make([]byte, l)
	for i := 0; i < l; i++ {
		bytes[i] = byte(randInt(97, 122))
	}
	return string(bytes)
}

// randInt generates a random integer
func randInt(min int, max int) int {
	return min + rand.Intn(max-min)
}

// Translate K8S Capacity to MapR Quota
func convertCapacity(capacity int64) string {
	Plog.Printf("INFO  Convert Kubernetes capacity: %s", capacity)
	mb := capacity / 1000000
	quota := strconv.FormatInt(mb, 10) + "M"
	Plog.Printf("INFO  Converted MapR capacity: %s", quota)
	return quota
}

// getSecretLocation gets location of admin secrets from params
func (p *maprProvisioner) getSecretLocation(cleanoptions map[string]string) (string, string) {
	sname := cleanoptions["maprSecretName"]
	snamespace := cleanoptions["maprSecretNamespace"]
	delete(cleanoptions, "maprSecretName")
	delete(cleanoptions, "maprSecretNamespace")
	return sname, snamespace
}

// getUserInfo gets UserInfo from secrets
func (p *maprProvisioner) getUserInfo(sname string, snamespace string) (url.Userinfo, error) {
	secretMap := make(map[string]string)
	username := ""
	password := ""
	// TODO: Replace with undocumented REST API using Admin Ticket
	//maprticket := ""
	userinfo := *url.UserPassword(username, password)
	if p.kubeClient == nil {
		txt := "ERROR  Cannot get kube client"
		Plog.Println(txt)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningUser", txt)
		return userinfo, errors.New(txt)
	}
	Plog.Printf("INFO  Getting admin secret: %s from namespace: %s", sname, snamespace)
	secrets, err := p.kubeClient.Core().Secrets(snamespace).Get(sname, metav1.GetOptions{})
	if err != nil {
		txt := "ERROR  Can't get secrets"
		Plog.Printf(txt)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningSecrets", txt)
		return userinfo, errors.New(txt)
	}
	for name, data := range secrets.Data {
		secretMap[name] = string(data)
	}
	if len(secretMap) == 0 {
		txt := "ERROR  Empty secret map data"
		Plog.Printf(txt)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningSecrets", txt)
		return userinfo, errors.New(txt)
	}
	for k, v := range secretMap {
		if k == adminUserKey {
			username = v
		}
		if k == adminPasswordKey {
			password = v
		}
		// TODO: Replace with undocumented REST API using Admin Ticket
		//if k == adminMaprTicketKey {
		//	password = v
		//}
	}
	userinfo = *url.UserPassword(username, password)
	Plog.Println("INFO  Got admin secret")
	p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "ProvisioningSecret", "Got admin secret")
	return userinfo, nil
}

// createServerInfo creates serverInfo from params
func (p *maprProvisioner) createServerInfo(cleanoptions map[string]string) serverInfo {
	restservers := cleanoptions["restServers"]
	cldb := cleanoptions["cldbHosts"]
	cluster := cleanoptions["cluster"]
	security := cleanoptions["securityType"]
	ticketSecretName := cleanoptions["ticketSecretName"]
	ticketSecretNamespace := cleanoptions["ticketSecretNamespace"]
	sname, snamespace := p.getSecretLocation(cleanoptions)
	userinfo, err := p.getUserInfo(sname, snamespace)
	if err != nil {
		Plog.Printf("ERROR  Failed to get User Info: %v", err)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "Provisioning", fmt.Sprintf("Failed to get User Info: %v", err))
	}
	si := serverInfo{REST: restservers, Cldbhosts: cldb, Cluster: cluster, Securitytype: security, Userinfo: userinfo, RESTSecretName: sname, RESTSecretNamespace: snamespace, TicketSecretName: ticketSecretName, TicketSecretNamespace: ticketSecretNamespace}
	delete(cleanoptions, "restServers")
	delete(cleanoptions, "cldbHosts")
	delete(cleanoptions, "cluster")
	delete(cleanoptions, "securityType")
	delete(cleanoptions, "ticketSecretName")
	delete(cleanoptions, "ticketSecretNamespace")
	return si
}

// createVolumeInfo creates volumeInfo from params
func (p *maprProvisioner) createVolumeInfo(cleanoptions map[string]string, quota string) *volumeInfo {
	platinum := cleanoptions["platinum"]
	if platinum == "" {
		platinum = "false"
	}
	ro := cleanoptions["readOnly"]
	if ro == "" {
		ro = "false"
	}
	mo := cleanoptions["mountOptions"]
	rp := cleanoptions["reclaimPolicy"]
	reclaim := v1.PersistentVolumeReclaimDelete
	if rp == "Retain" {
		reclaim = v1.PersistentVolumeReclaimRetain
	}
	namePrefix := cleanoptions["namePrefix"]
	if namePrefix == "" {
		namePrefix = "maprprovisioner"
	}
	mountPrefix := cleanoptions["mountPrefix"]
	uniqueName := randomString(randomSize)
	pvname := namePrefix + "-" + uniqueName
	volumename := namePrefix + "." + uniqueName
	volumepath := mountPrefix + "/" + pvname
	cleanoptions["name"] = volumename
	cleanoptions["path"] = volumepath
	cleanoptions["quota"] = quota
	delete(cleanoptions, "namePrefix")
	delete(cleanoptions, "mountPrefix")
	delete(cleanoptions, "platinum")
	delete(cleanoptions, "mountOptions")
	delete(cleanoptions, "readOnly")
	delete(cleanoptions, "reclaimPolicy")
	// we have to always create parent since the volume needs to be mountable
	// so if prefix doesnt exist we will create. We have to ignore what they passed in
	cleanoptions["createparent"] = "1"
	// always mount volume
	cleanoptions["mount"] = "1"
	vi := volumeInfo{PVName: pvname, Name: volumename, Path: volumepath, Quota: quota, Platinum: platinum, ReadOnly: ro, MountOptions: mo, ReclaimPolicy: reclaim}
	return &vi
}

// cleanParams sanitizes StorageClass input
func (p *maprProvisioner) cleanParams(options controller.VolumeOptions) map[string]string {
	cleanoptions := make(map[string]string)
	params := options.Parameters
	legalMap := make(map[string]bool)
	for i := 0; i < len(legalparams); i++ {
		legalMap[legalparams[i]] = true
	}
	for k := range params {
		ok, _ := legalMap[k]
		if ok == true {
			cleanoptions[k] = params[k]
		} else {
			Plog.Printf("WARNING  Parameters contained illegal param: %s with value: %s", k, params[k])
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "Provisioning", fmt.Sprintf("Parameters contained illegal param: %s with value: %s", k, params[k]))
		}
	}
	return cleanoptions
}

// createRawQuery builds REST parameter string with MapR options
func (p *maprProvisioner) createRawQuery(cleanoptions map[string]string) string {
	var buffer bytes.Buffer
	for k := range cleanoptions {
		buffer.WriteString("&")
		buffer.WriteString(k)
		buffer.WriteString("=")
		buffer.WriteString(url.QueryEscape(cleanoptions[k]))
	}
	rawquery := buffer.String()
	rawquery = strings.TrimLeft(rawquery, "&")
	return rawquery
}

// constructQuery builds REST query for Maprfs
func (p *maprProvisioner) constructQuery(restserver string, userinfo *url.Userinfo, path string, rawquery string) url.URL {
	u, err := url.Parse("http://example.com")
	if err != nil {
		Plog.Println(err)
	}
	u.Scheme = "https"
	u.Host = restserver
	u.User = userinfo
	u.Path = path
	u.RawQuery = rawquery
	return *u
}

// executeQuery calls MapR via REST interface
func (p *maprProvisioner) executeQuery(query url.URL) (*http.Response, error) {
	w := os.Stdout
	// TODO: Take a param to verify via cert authority
	client := &http.Client{
		Transport: &http.Transport{
			IdleConnTimeout:    60 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			TLSClientConfig: &tls.Config{
				KeyLogWriter:       w,
				InsecureSkipVerify: true,
			},
		},
	}
	qstring := query.String()
	req, err := http.NewRequest("POST", qstring, nil)
	if err != nil {
		Plog.Printf("ERROR  Could not create request: %v", err)
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		Plog.Printf("ERROR  Failed to get URL: %v", err)
		return nil, err
	}
	return resp, nil
}

func (p *maprProvisioner) createMaprVolume(cleanoptions map[string]string, si serverInfo) error {
	maprquerystring := "/rest/volume/create"
	rawquery := p.createRawQuery(cleanoptions)
	restservers := strings.Split(si.REST, " ")
	success := false
	for idx := range restservers {
		Plog.Println("INFO  Creating MapR query...")
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", "Creating MapR query")
		restserver := restservers[idx]
		if !strings.Contains(restserver, ":") {
			restserver = restserver + restServerPort
		}
		query := p.constructQuery(restserver, &si.Userinfo, maprquerystring, rawquery)
		Plog.Printf("INFO  Calling executeQuery with query string: %s", maprquerystring+"?"+rawquery)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", fmt.Sprintf("Calling executeQuery with query string: %s", maprquerystring+"?"+rawquery))
		resp, err := p.executeQuery(query)
		if err != nil {
			Plog.Printf("ERROR  Error executing query: %v", err)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningError", fmt.Sprintf("Error executing query: %v", err))
			continue
		}
		if resp.StatusCode <= 200 && resp.StatusCode >= 299 {
			Plog.Printf("ERROR  Authorization Error: %v", err)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningError", fmt.Sprintf("Authorization Error: %v", err))
			break
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Plog.Printf("ERROR  Failed to get URL: %v", err)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningError", fmt.Sprintf("Failed to get URL: %v", err))
			continue
		} else {
			Plog.Printf("INFO  Response: %s", body)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "ProvisioningREST", fmt.Sprintf("Response: %s", body))
			var options map[string]interface{}
			json.Unmarshal([]byte(body), &options)
			status := options["status"]
			if status == "ERROR" {
				Plog.Printf("Failed to get URL: %v", err)
				p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningError", fmt.Sprintf("ERROR  Failed to get URL: %v", err))
			} else {
				success = true
				break
			}
		}
	}
	if !success {
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningError", "Cannot create volume")
		return errors.New("ERROR  cannot create volume")
	}
	return nil
}

func (p *maprProvisioner) deleteMaprVolume(volumename string, si serverInfo) error {
	maprquerystring := "/rest/volume/remove"
	rawquery := "name=" + volumename
	restservers := strings.Split(si.REST, " ")
	success := false
	for idx := range restservers {
		Plog.Println("INFO  Creating MapR query...")
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "DeleteVolume", "Creating MapR query")
		restserver := restservers[idx]
		if !strings.Contains(restserver, ":") {
			restserver = restserver + restServerPort
		}
		query := p.constructQuery(restserver, &si.Userinfo, maprquerystring, rawquery)
		Plog.Printf("INFO  Calling executeQuery with query string: %s", maprquerystring+"?"+rawquery)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "DeleteREST", fmt.Sprintf("Calling executeQuery with query string: %s", maprquerystring+"?"+rawquery))
		resp, err := p.executeQuery(query)
		if err != nil {
			Plog.Printf("ERROR  Error executing query: %v", err)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Error executing query: %v", err))
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode <= 200 && resp.StatusCode >= 299 {
			Plog.Printf("ERROR  Authorization Error: %v", err)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Authorization Error: %v", err))
			break
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Plog.Printf("ERROR  Failed to get URL: %v", err)
			p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Failed to get URL: %v", err))
			continue
		} else {
			Plog.Printf("INFO  Response: %s", body)
			var options map[string]interface{}
			json.Unmarshal([]byte(body), &options)
			status := options["status"]
			if status == "ERROR" {
				Plog.Printf("ERROR  Failed to get URL: %v", err)
				p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Failed to get URL: %v", err))
			} else {
				success = true
				break
			}
		}
	}
	if !success {
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "DeleteError", "Cannot delete volume")
		return errors.New("ERROR  cannot delete volume")
	}
	return nil
}

// NewMaprProvisioner creates a new instance of a MapR provisioner
func NewMaprProvisioner(client kubernetes.Interface, id string) controller.Provisioner {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: client.CoreV1().Events(v1.NamespaceAll)})
	var eventRecorder record.EventRecorder
	out, err := os.Hostname()
	if err != nil {
		Plog.Println("ERROR  Error getting hostname for specifying it as source of events: %v", err)
		eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: provisionerName})
	} else {
		eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("%s-%s", provisionerName, out)})
	}
	return &maprProvisioner{
		kubeClient:    client,
		identity:      id,
		eventRecorder: eventRecorder,
	}
}

var _ controller.Provisioner = &maprProvisioner{}

// Provision creates a MapRfs volume and returns a PV object representing it.
func (p *maprProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	Plog.Println("INFO  === Starting volume provisioning ===")
	Plog.Printf("INFO  options=%v", options)
	Plog.Println("INFO  Cleaning parameters...")
	p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", "Cleaning parameters")
	p.eventTarget = options.PVC
	cleanoptions := p.cleanParams(options)
	Plog.Println("INFO  Parsing parameters...")
	p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", "Parsing parameters")
	si := p.createServerInfo(cleanoptions)
	Plog.Printf("INFO  Constructed server info: (rest: %s, cldb: %s, cluster: %s, securitytype: %s)", si.REST, si.Cldbhosts, si.Cluster, si.Securitytype)
	p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", fmt.Sprintf("Constructed server info: (rest: %s, cldb: %s, cluster: %s, securitytype: %s)", si.REST, si.Cldbhosts, si.Cluster, si.Securitytype))
	capQty := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	capacity := capQty.Value()
	quota := convertCapacity(capacity)
	vi := p.createVolumeInfo(cleanoptions, quota)
	Plog.Printf("INFO  Generated Mapr volumename: %s mountpoint: %s", vi.Name, vi.Path)
	p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", fmt.Sprintf("Generated Mapr volumename: %s mountpoint: %s", vi.Name, vi.Path))
	err := p.createMaprVolume(cleanoptions, si)
	if err != nil {
		txt := "Can't create volume because of REST error"
		Plog.Println("ERROR  " + txt)
		p.eventRecorder.Event(p.eventTarget, v1.EventTypeWarning, "ProvisioningError", txt)
		return nil, errors.New(txt)
	}
	Plog.Printf("INFO  Creating Kubernetes PersistentVolume: %s", vi.PVName)
	p.eventRecorder.Event(p.eventTarget, v1.EventTypeNormal, "Provisioning", fmt.Sprintf("Creating Kubernetes PersistentVolume: %s", vi.PVName))
	// TODO: Change to standard reclaimPolicy when K8S fixes
	// reclaim := options.PersistentVolumeReclaimPolicy
	reclaim := vi.ReclaimPolicy
	Plog.Printf("INFO  Reclaim Policy: %v", reclaim)
	description := "Dynamically provisioned PV for MapR-FS: " + vi.Name
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: vi.PVName,
			Annotations: map[string]string{
				"mapr.com/maprProvisionerIdentity": p.identity,
				"mapr.com/provisionerVersion":      provisionerVersion,
				"mapr.com/description":             description,
				"mapr.com/restServers":             si.REST,
				"mapr.com/secretName":              si.RESTSecretName,
				"mapr.com/secretNamespace":         si.RESTSecretNamespace,
				"mapr.com/volumeName":              vi.Name,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: reclaim,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): capQty,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexVolumeSource{
					Driver: driverName,
					Options: map[string]string{
						"volumePath":            vi.Path,
						"cluster":               si.Cluster,
						"cldbHosts":             si.Cldbhosts,
						"securityType":          si.Securitytype,
						"ticketSecretName":      si.TicketSecretName,
						"ticketSecretNamespace": si.TicketSecretNamespace,
						"platinum":              vi.Platinum,
						"mountOptions":          vi.MountOptions,
						"readOnly":              vi.ReadOnly,
					},
				},
			},
		},
	}
	Plog.Println("INFO  === Finished volume provisioning ===")
	return pv, nil
}

// Delete removes MapRfs Volume
func (p *maprProvisioner) Delete(volume *v1.PersistentVolume) error {

	p.eventTarget = volume
	_, ok := volume.Annotations["mapr.com/maprProvisionerIdentity"]
	if !ok {
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", "identity annotation on PV is not mapr.com/maprProvisionerIdentity")
		return &controller.IgnoredError{Reason: "identity annotation on PV is not mapr.com/maprProvisionerIdentity"}
	}
	Plog.Println("INFO  === Starting volume delete ===")
	restservers, ok := volume.Annotations["mapr.com/restServers"]
	if !ok {
		txt := "mapr.com/restServers annotation not found on PV"
		Plog.Println("ERROR  " + txt)
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", txt)
		return errors.New(txt)
	}
	volumename, ok := volume.Annotations["mapr.com/volumeName"]
	if !ok {
		txt := "mapr.com/volumeName annotation not found on PV"
		Plog.Println("ERROR  " + txt)
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", txt)
		return errors.New(txt)
	}
	sname, ok := volume.Annotations["mapr.com/secretName"]
	if !ok {
		txt := "mapr.com/secretName annotation not found on PV"
		Plog.Println("ERROR  " + txt)
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", txt)
		return errors.New(txt)
	}
	snamespace, ok := volume.Annotations["mapr.com/secretNamespace"]
	if !ok {
		txt := "mapr.com/secretNamespace annotation not found on PV"
		Plog.Println("ERROR  " + txt)
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", txt)
		return errors.New(txt)
	}
	Plog.Printf("INFO  Creating MapR query...")
	userinfo, err := p.getUserInfo(sname, snamespace)
	if err != nil {
		Plog.Printf("ERROR  Failed to get User Info: %v", err)
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", fmt.Sprintf("Failed to get User Info: %v", err))
		return errors.New("Failed to get User Info")
	}
	si := serverInfo{REST: restservers, Userinfo: userinfo}
	err = p.deleteMaprVolume(volumename, si)
	if err != nil {
		txt := "Failed delete MapR via REST"
		Plog.Println("ERROR  " + txt)
		p.eventRecorder.Event(volume, v1.EventTypeWarning, "DeleteVolumeWarning", txt)
		return errors.New(txt)
	}
	Plog.Println("INFO  === Finished volume delete ===")
	return nil
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	syscall.Umask(0)

	flag.Parse()
	flag.Set("logtostderr", "true")

	var config *rest.Config
	var err error
	f, err := os.OpenFile(provision_log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Errorf("ERROR  Can't create provisioner log! Reason: %v \n", err)
		return
	}
	defer f.Close()
	Plog = log.New(f, "", log.Ldate|log.Ltime|log.Lshortfile)
	Plog.Println("INFO  Starting provisioner...")
	if *master != "" || *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		Plog.Println("ERROR  Failed to create config: %v", err)
		return
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		Plog.Printf("ERROR  Failed to create client: %v", err)
		return
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		Plog.Printf("ERROR  Error getting server version... %v", err)
		return
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	maprProvisioner := NewMaprProvisioner(clientset, provisionerName)

	// Start the provision controller which will dynamically provision Mapr
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, maprProvisioner, serverVersion.GitVersion)
	pc.Run(wait.NeverStop)
}
