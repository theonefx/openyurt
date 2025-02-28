/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/controller/nodepool"
	"github.com/openyurtio/openyurt/pkg/controller/raven"
	"github.com/openyurtio/openyurt/pkg/controller/staticpod"
	ctrlutil "github.com/openyurtio/openyurt/pkg/controller/util"
	"github.com/openyurtio/openyurt/pkg/controller/yurtappdaemon"
	"github.com/openyurtio/openyurt/pkg/controller/yurtappset"
	v1alpha1gateway "github.com/openyurtio/openyurt/pkg/webhook/gateway/v1alpha1"
	v1alpha1nodepool "github.com/openyurtio/openyurt/pkg/webhook/nodepool/v1alpha1"
	v1beta1nodepool "github.com/openyurtio/openyurt/pkg/webhook/nodepool/v1beta1"
	v1pod "github.com/openyurtio/openyurt/pkg/webhook/pod/v1"
	v1alpha1staticpod "github.com/openyurtio/openyurt/pkg/webhook/staticpod/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/webhook/util"
	webhookcontroller "github.com/openyurtio/openyurt/pkg/webhook/util/controller"
	"github.com/openyurtio/openyurt/pkg/webhook/util/health"
	v1alpha1yurtappdaemon "github.com/openyurtio/openyurt/pkg/webhook/yurtappdaemon/v1alpha1"
	v1alpha1yurtappset "github.com/openyurtio/openyurt/pkg/webhook/yurtappset/v1alpha1"
)

type SetupWebhookWithManager interface {
	// mutate path, validatepath, error
	SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error)
}

// controllerWebhooks is used to control whether enable or disable controller-webhooks
var controllerWebhooks map[string][]SetupWebhookWithManager

// independentWebhooks is used to control whether disable independent-webhooks
var independentWebhooks = make(map[string]SetupWebhookWithManager)

var WebhookHandlerPath = make(map[string]struct{})

func addControllerWebhook(name string, handler SetupWebhookWithManager) {
	if controllerWebhooks == nil {
		controllerWebhooks = make(map[string][]SetupWebhookWithManager)
	}

	if controllerWebhooks[name] == nil {
		controllerWebhooks[name] = make([]SetupWebhookWithManager, 0)
	}

	controllerWebhooks[name] = append(controllerWebhooks[name], handler)
}

func init() {
	addControllerWebhook(raven.ControllerName, &v1alpha1gateway.GatewayHandler{})
	addControllerWebhook(nodepool.ControllerName, &v1alpha1nodepool.NodePoolHandler{})
	addControllerWebhook(nodepool.ControllerName, &v1beta1nodepool.NodePoolHandler{})
	addControllerWebhook(staticpod.ControllerName, &v1alpha1staticpod.StaticPodHandler{})
	addControllerWebhook(yurtappset.ControllerName, &v1alpha1yurtappset.YurtAppSetHandler{})
	addControllerWebhook(yurtappdaemon.ControllerName, &v1alpha1yurtappdaemon.YurtAppDaemonHandler{})

	independentWebhooks["pod"] = &v1pod.PodHandler{}
}

// Note !!! @kadisi
// Do not change the name of the file or the contents of the file !!!!!!!!!!
// Note !!!

func SetupWithManager(c *config.CompletedConfig, mgr manager.Manager) error {
	setup := func(s SetupWebhookWithManager) error {
		m, v, err := s.SetupWebhookWithManager(mgr)
		if err != nil {
			return fmt.Errorf("unable to create webhook %v", err)
		}
		if _, ok := WebhookHandlerPath[m]; ok {
			panic(fmt.Errorf("webhook handler path %s duplicated", m))
		}
		WebhookHandlerPath[m] = struct{}{}
		klog.Infof("Add webhook mutate path %s", m)
		if _, ok := WebhookHandlerPath[v]; ok {
			panic(fmt.Errorf("webhook handler path %s duplicated", v))
		}
		WebhookHandlerPath[v] = struct{}{}
		klog.Infof("Add webhook validate path %s", v)

		return nil
	}

	// set up independent webhooks
	for name, s := range independentWebhooks {
		if util.IsWebhookDisabled(name, c.ComponentConfig.Generic.DisabledWebhooks) {
			klog.Warningf("Webhook %v is disabled", name)
			continue
		}
		if err := setup(s); err != nil {
			return err
		}
	}

	// set up controller webhooks
	for controllerName, list := range controllerWebhooks {
		if !ctrlutil.IsControllerEnabled(controllerName, c.ComponentConfig.Generic.Controllers) {
			klog.Warningf("Webhook for %v is disabled", controllerName)
			continue
		}
		for _, s := range list {
			if err := setup(s); err != nil {
				return err
			}
		}
	}
	return nil
}

type GateFunc func() (enabled bool)

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update;patch

func Initialize(ctx context.Context, cfg *rest.Config, cc *config.CompletedConfig) error {
	c, err := webhookcontroller.New(cfg, WebhookHandlerPath, cc)
	if err != nil {
		return err
	}
	go func() {
		c.Start(ctx)
	}()

	timer := time.NewTimer(time.Second * 20)
	defer timer.Stop()
	select {
	case <-webhookcontroller.Inited():
		return nil
	case <-timer.C:
		return fmt.Errorf("failed to start webhook controller for waiting more than 20s")
	}
}

func Checker(req *http.Request) error {
	// Firstly wait webhook controller initialized
	select {
	case <-webhookcontroller.Inited():
	default:
		return fmt.Errorf("webhook controller has not initialized")
	}
	return health.Checker(req)
}

func WaitReady() error {
	startTS := time.Now()
	var err error
	for {
		duration := time.Since(startTS)
		if err = Checker(nil); err == nil {
			return nil
		}

		if duration > time.Second*5 {
			klog.Warningf("Failed to wait webhook ready over %s: %v", duration, err)
		}
		time.Sleep(time.Second * 2)
	}

}
