# Persistent Volume Resizer Controller
This controller is used to Resize the Persistent Volume in a kubernetes cluster.

# Run in local Machine
## Setup
- To run this controller at my local machine I have used [kind](https://kind.sigs.k8s.io/) cluster along with podman.
-- As in  a local development cluster like kind, the default local-path provisioner does not support volume expansion. I have used [csi-driver-host-path](https://github.com/kubernetes-csi/csi-driver-host-path) to support that. 
- See the detailed install steps [here](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/docs/deploy-1.17-and-later.md)

- Create some csi-hostpath based storage class like the following:

```
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-hostpath-sc
provisioner: hostpath.csi.k8s.io
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
```

This storage class needs to be used in pvc.

## Build and Deployment
- Build the image with:
```
$ make docker-build
```
This will make an image `localhost/makro/pvc-resizer:latest` at podman.

- Deploy: But we can't use this image directly at kind cluster, this image needs to be imported at kind cluster. To do that, I have made an new target at makefile:

```
$ make kind-import-image
```

which will basically tag the image as `docker.io/makro/pvc-resizer:latest` and import it to kind cluster.
Alternately, we can use some common registry dockerhub, aws ecr etc.

Now we can deploy by:
```
$ make deploy
```

That will deploy a `Deployment` resource having the controller image used in pod and all other needed resources to the kind cluster and that's it now its ready to use.

## Try out
- To try it, you can define some pvc like the following:

```
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-pvc1
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-hostpath-sc 
```
And define some pvcResize like:

```
apiVersion: volume.makro.com/v1alpha1
kind: PVCResize
metadata:
  labels:
    app.kubernetes.io/name: pvc-resizer-controller
    app.kubernetes.io/managed-by: kustomize
  name: sample1
spec:
  policies:
    - ref: csi-pvc1
      size: 3Gi
```

After a while, you will se the size of the pvc `csi-pvc1` has been increased to 3Gi and its done by the controller.
You can also check the details at the status of the PVCResize resource.

# Demo
To get more insight, please have a look at my Demo from my local machine using kind cluster: 
https://asciinema.org/a/QTrF0FtYzS6c0IcIYIyQfhERu