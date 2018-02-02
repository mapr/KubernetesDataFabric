To install plugin and provisioner:

  1. kubectl create -f maprfs-namespace.yaml
  2. kubectl create -f maprfs-RBAC.yaml
  3. Either kubectl create -f maprfs-plugin-centos.yaml (for RedHat,Centos and SUSE host environments) or kubectl create -f maprfs-plugin-ubuntu.yaml (for Ubuntu Environments)
  4. kubectl create -f maprfs-provisioner.yaml

Examples:

  1. Mounting a statically provisioned MaprFS volume
  (change securityType to unsecure and remove ticket if you dont have a secure Mapr cluster)

  apiVersion: v1
  kind: Pod
  metadata:
    name: maprfs-volume-example
  spec:
    ...
    volumeMounts:
    - mountPath: /mapr
      name: examplemount
   volumes:
    - name: examplemount
      flexVolume:
        driver: "mapr.com/maprfs"
        options:
          volumePath: "/path/to/my/mapr/volume"
          cluster: "your.cluster.name"
          cldbHosts: "your.cldb.host1 your.cldb.host2"
          securityType: "secure"
          ticket: <BASE64 ENCODED VERSION OF CONTENTS OF TICKET FILE>

    2. Mounting a statically provisioned MaprFS Persistent Volume.
    (change securityType to unsecure and remove ticket if you dont have a secure Mapr cluster)

    apiVersion: v1
    kind: Pod
    metadata:
      name: PV-example
    spec:
      ...
      volumeMounts:
      - mountPath: /mapr
        name: examplemount
     Volumes:
       - name: examplemount
         persistentVolumeClaim:
           claimName: PV-example
     ---
     kind: PersistentVolumeClaim
     apiVersion: v1
     metadata:
       name: PV-example
     spec:
       accessModes:
         - ReadWriteOnce
       resources:
         requests:
           storage: 5G
      ---
      apiVersion: v1
      kind: PersistentVolume
      metadata:
        name: PV-example
      spec:
        capacity:
          storage: 5Gi
        accessModes:
          - ReadWriteOnce
        flexVolume:
          driver: "mapr.com/maprfs"
          options:
            volumePath: "/path/to/my/mapr/volume"
            cluster: "your.cluster.name"
            cldbHosts: "your.cldb.host1 your.cldb.host2"
            securityType: "secure"
            ticket: <BASE64 ENCODED VERSION OF CONTENTS OF TICKET FILE>

      3. Mounting dynamically provisioned MaprFS Persistent Volumes.
      (change securityType to unsecure and remove ticket if you dont have a secure Mapr cluster)

      apiVersion: v1
      kind: Pod
      metadata:
        name: provisioner-example
      spec:
        ...
        volumeMounts:
        - mountPath: /mapr
          name: examplemount
       Volumes:
         - name: examplemount
           persistentVolumeClaim:
             claimName: maprclaim-example
       ---
       kind: PersistentVolumeClaim
       apiVersion: v1
       metadata:
         name: maprclaim-example
         annotations:
           volume.beta.kubernetes.io/storage-class: "example-storageclass"
       spec:
         accessModes:
           - ReadWriteOnce
         resources:
           requests:
             storage: 5G
        ---
        apiVersion: storage.k8s.io/v1
        kind: StorageClass
        metadata:
           name: example-storageclass
        provisioner: mapr.com/maprfs
        parameters:
            webserverLocation: "your.mapr.webserver:8443"
            cldbHosts: "your.cldb.host1 your.cldb.host2"
            cluster: "your.cluster.name"
            securityType: "secure"
            maprSecretName: "your-volumeadmin-secrets"
            maprSecretNamespace: "your-secret-namespace"
            ticket: <BASE64 ENCODED VERSION OF CONTENTS OF TICKET FILE>
            namePrefix: "yourname" // a prefix appended to all dynamic volume names
            mountPrefix: "/yourname" // mountpoint where new volumes are mounted
            type: "rw"
            advisoryquota: "100M"
            quota: "500M"
