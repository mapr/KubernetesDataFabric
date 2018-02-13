Most Pods in a Kubernetes environment should be portable, short-lived, and stateless. Traditionally, when a Pod is stopped or moved, the state of its containers could be lost. The MapR Data Fabric for Kubernetes:
- Provides long-lived, persistent storage for Pods and their containers.
- Allows containers running in Kubernetes to use the MapR filesystem for all of their storage needs.
- Allows secure storage of all container states in MapR-XD.

The MapR Data Fabric for Kubernetes consists of a set of Docker containers and their respective .yaml configuration files for installation into Kubernetes. Once installed, both a Kubernetes FlexVolume Driver for MaprFS and a Kubernetes Dynamic Volume Provisioner are available for both static and dynamic provisioning of MapR storage.

The structure of this project is as follows:
- The build folder contains the docker images to build the  MapR Data Fabric for Kubernetes
- The deploy folder contains the YAML files for product installation in Kubernetes
- The examples folder contains a set of example YAML files that create pods connecting to the data fabric 
