This is a hack to redirect klog to zap.

It is originally based on https://github.com/istio/klog/blob/master/klog.go, with some modifications to add missing methods.

This code is copied to the vendor directory as it is a seperate module, if you make changes here you need to run go mod vendor to make them visible to the controller.


