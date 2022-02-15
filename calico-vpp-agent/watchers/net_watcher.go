// Copyright (C) 2021 Cisco Systems Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package watchers

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	v3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	"github.com/projectcalico/api/pkg/client/informers_generated/externalversions"
	"github.com/projectcalico/vpp-dataplane/calico-vpp-agent/common"
	"github.com/projectcalico/vpp-dataplane/vpplink"
	"github.com/sirupsen/logrus"
	"gopkg.in/tomb.v2"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type VRF struct {
	Tables [2]uint32 // one for ipv4, one for ipv6
}

type NetworkDefinition struct {
	VRF  VRF
	Vni  uint32
	Name string
}

type NetWatcher struct {
	log                *logrus.Entry
	vpp                *vpplink.VppLink
	client             *clientset.Clientset
	stop               chan struct{}
	factory            externalversions.SharedInformerFactory
	informer           cache.SharedIndexInformer
	networkDefinitions map[string]NetworkDefinition
}

func NewNetWatcher(vpp *vpplink.VppLink, log *logrus.Entry) *NetWatcher {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	client, err := clientset.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	factory := externalversions.NewSharedInformerFactory(client, 10*time.Minute)
	informer := factory.Projectcalico().V3().Networks().Informer()

	w := NetWatcher{
		log:                log,
		vpp:                vpp,
		client:             client,
		stop:               make(chan struct{}),
		factory:            factory,
		informer:           informer,
		networkDefinitions: make(map[string]NetworkDefinition),
	}
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			net, ok := obj.(*v3.Network)
			if !ok {
				w.log.Errorf("Wrong object type received in network watcher: %v", obj)
			}
			w.OnNetAdded(net)
		},
		UpdateFunc: func(old, new interface{}) {
			oldNet, ok := old.(*v3.Network)
			if !ok {
				w.log.Errorf("Wrong object type received in network watcher: %v", old)
			}
			newNet, ok := new.(*v3.Network)
			if !ok {
				w.log.Errorf("Wrong object type received in network watcher: %v", new)
			}
			w.OnNetChanged(oldNet, newNet)
		},
		DeleteFunc: func(obj interface{}) {
			net, ok := obj.(*v3.Network)
			if !ok {
				w.log.Errorf("Wrong object type received in network watcher: %v", obj)
			}
			w.OnNetDeleted(net)
		},
	})

	return &w
}

func (w *NetWatcher) OnVppRestart() {
	/* We don't do anything */
}

func (w *NetWatcher) WatchNetworks(t *tomb.Tomb) error {
	w.log.Infof("Net watcher starts")
	w.factory.Start(w.stop)
	return nil
}

func (w *NetWatcher) Stop() {
	close(w.stop)
}

func (w *NetWatcher) OnNetAdded(net *v3.Network) error {
	w.log.Infof("adding network %s", net.Name)
	netDef, err := w.CreateVRFsForNet(net)
	if err != nil {
		return err
	}
	common.SendEvent(common.CalicoVppEvent{
		Type: common.NetAdded,
		New:  netDef,
	})
	return nil
}

func (w *NetWatcher) OnNetChanged(old, new *v3.Network) {
	w.log.Infof("network %s changed", old.Name)
}

func (w *NetWatcher) OnNetDeleted(net *v3.Network) error {
	w.log.Infof("deleting network %s", net.Name)
	netDef, err := w.DeleteNetVRFs(net)
	if err != nil {
		return err
	}
	common.SendEvent(common.CalicoVppEvent{
		Type: common.NetDeleted,
		Old:  netDef,
	})
	return nil
}

func getNetworkVrfName(networkName string, suffix string) string {
	return fmt.Sprintf("pod-%s-table-%s", networkName, suffix)
}

func (w *NetWatcher) CreateVRFsForNet(network *v3.Network) (netDef *NetworkDefinition, err error) {
	/* Create and Setup the per-network VRF */
	var tables [2]uint32
	if _, ok := w.networkDefinitions[network.Name]; ok {
		return nil, errors.Errorf("existing network added: %s", network.Name)
	}
	for idx, ipFamily := range vpplink.IpFamilies {
		vrfName := getNetworkVrfName(network.Name, ipFamily.Str)
		vrfId, err := w.vpp.AllocateVRF(ipFamily.IsIp6, vrfName)
		w.log.Debugf("Allocated %s VRF ID:%d", ipFamily.Str, vrfId)
		if err != nil {
			return nil, errors.Wrapf(err, "error allocating VRF %s", ipFamily.Str)
		}

		/*s.log.Infof("Adding network VRF %d %s default route via VRF %d", vrfId, ipFamily.Str, common.PodVRFIndex)
		err = s.vpp.AddDefaultRouteViaTable(vrfId, common.PodVRFIndex, ipFamily.IsIp6)
		if err != nil {
			return errors.Wrapf(err, "error adding VRF %d %s default route via VRF %d", vrfId, ipFamily.Str, common.PodVRFIndex)
		}*/

		tables[idx] = vrfId
	}
	netDef = &NetworkDefinition{VRF: VRF{Tables: tables}, Vni: uint32(network.Spec.VNI), Name: network.Name}
	w.networkDefinitions[network.Name] = *netDef
	return netDef, nil
}

func (w *NetWatcher) DeleteNetVRFs(network *v3.Network) (*NetworkDefinition, error) {
	var err error
	if _, ok := w.networkDefinitions[network.Name]; !ok {
		w.log.Errorf("non-existent network deleted: %s", network.Name)
	}
	for idx, ipFamily := range vpplink.IpFamilies {
		vrfId := w.networkDefinitions[network.Name].VRF.Tables[idx]
		/*s.log.Infof("Deleting VRF %d %s default route via VRF %d", vrfId, ipFamily.Str, common.PodVRFIndex)
		err = s.vpp.DelDefaultRouteViaTable(vrfId, common.PodVRFIndex, ipFamily.IsIp6)
		if err != nil {
			s.log.Errorf("Error  VRF %d %s default route via VRF %d : %s", vrfId, ipFamily.Str, common.PodVRFIndex, err)
		}*/
		w.log.Infof("Deleting VRF %d %s", vrfId, ipFamily.Str)
		err = w.vpp.DelVRF(vrfId, ipFamily.IsIp6, network.Name)
		if err != nil {
			w.log.Errorf("Error deleting VRF %d %s : %s", vrfId, ipFamily.Str, err)
		}
	}
	netDef := w.networkDefinitions[network.Name]
	delete(w.networkDefinitions, network.Name)
	return &netDef, nil
}
