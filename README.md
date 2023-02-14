## Usage Example

### Server
```
$ make && sudo rm /tmp/foo && sudo bin/libvirtproxy /tmp/foo
```

### Client
```
$ sudo virsh -c qemu:///system?socket=/tmp/foo dumpxml focal
```
