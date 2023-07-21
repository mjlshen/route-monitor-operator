package servicemonitor

import (
	"context"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	util "github.com/openshift/route-monitor-operator/pkg/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhobsv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceMonitor struct {
	Client   client.Client
	Comparer util.ResourceComparerInterface
}

func NewServiceMonitor(c client.Client) *ServiceMonitor {
	return &ServiceMonitor{
		Client:   c,
		Comparer: &util.ResourceComparer{},
	}
}

const (
	ServiceMonitorPeriod string = "30s"
	UrlLabelName         string = "probe_url"
)

func (u *ServiceMonitor) TemplateAndUpdateServiceMonitorDeployment(ctx context.Context, routeURL, blackBoxExporterNamespace string, namespacedName types.NamespacedName, clusterID string, isHCPMonitor bool) error {
	params := map[string][]string{
		// Currently we only support `http_2xx` as module
		"module": {"http_2xx"},
		"target": {routeURL},
	}

	if isHCPMonitor {
		s := u.HyperShiftTemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace, params, namespacedName, clusterID)
		return u.HypershiftUpdateServiceMonitorDeployment(ctx, s)
	}
	s := u.TemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace, params, namespacedName, clusterID)
	return u.UpdateServiceMonitorDeployment(ctx, s)
}

// Creates or Updates Service Monitor Deployment according to the template
func (u *ServiceMonitor) UpdateServiceMonitorDeployment(ctx context.Context, template monitoringv1.ServiceMonitor) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &monitoringv1.ServiceMonitor{}
	err := u.Client.Get(ctx, namespacedName, deployedServiceMonitor)
	if err != nil {
		// No similar ServiceMonitor exists
		if !kerr.IsNotFound(err) {
			return err
		}
		return u.Client.Create(ctx, &template)
	}
	if !u.Comparer.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return u.Client.Update(ctx, deployedServiceMonitor)
	}
	return nil
}

// Creates or Updates Service Monitor Deployment according to the template if enable of the hypershift
func (u *ServiceMonitor) HypershiftUpdateServiceMonitorDeployment(ctx context.Context, template rhobsv1.ServiceMonitor) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &rhobsv1.ServiceMonitor{}
	err := u.Client.Get(ctx, namespacedName, deployedServiceMonitor)
	if err != nil {
		// No similar ServiceMonitor exists
		if !kerr.IsNotFound(err) {
			return err
		}
		return u.Client.Create(ctx, &template)
	}
	if !u.Comparer.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return u.Client.Update(ctx, deployedServiceMonitor)
	}
	return nil
}

// DeleteServiceMonitorDeployment deletes the corresponding ServiceMonitor
// servicemonitor.monitoring.rhobs for HCP or
// servicemonitor.monitoring.coreos.com otherwise
func (u *ServiceMonitor) DeleteServiceMonitorDeployment(ctx context.Context, serviceMonitorRef v1alpha1.NamespacedName, isHCP bool) error {
	if serviceMonitorRef.Name == "" || serviceMonitorRef.Namespace == "" {
		return nil
	}
	namespacedName := types.NamespacedName{Name: serviceMonitorRef.Name, Namespace: serviceMonitorRef.Namespace}

	if isHCP {
		resource := &rhobsv1.ServiceMonitor{}
		if err := u.Client.Get(ctx, namespacedName, resource); err != nil {
			return client.IgnoreNotFound(err)
		}
		return u.Client.Delete(ctx, resource)
	}

	resource := &monitoringv1.ServiceMonitor{}
	if err := u.Client.Get(ctx, namespacedName, resource); err != nil {
		return client.IgnoreNotFound(err)
	}

	return u.Client.Delete(ctx, resource)
}

// TemplateForServiceMonitorResource returns a ServiceMonitor
func (u *ServiceMonitor) TemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace string, params map[string][]string, namespacedName types.NamespacedName, clusterID string) monitoringv1.ServiceMonitor {
	return monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackboxexporter.BlackBoxExporterPortName,
					// Probe every 30s
					Interval: monitoringv1.Duration(ServiceMonitorPeriod),
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Replacement: routeURL,
							TargetLabel: UrlLabelName,
						},
						{
							Replacement: clusterID,
							TargetLabel: "_id",
						},
					},
				}},
			Selector: metav1.LabelSelector{
				MatchLabels: blackboxexporter.GenerateBlackBoxExporterLables(),
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{
					blackBoxExporterNamespace,
				},
			},
		},
	}
}

// HyperShiftTemplateForServiceMonitorResource returns a ServiceMonitor for Hypershift
func (u *ServiceMonitor) HyperShiftTemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace string, params map[string][]string, namespacedName types.NamespacedName, clusterID string) rhobsv1.ServiceMonitor {
	return rhobsv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: rhobsv1.ServiceMonitorSpec{
			Endpoints: []rhobsv1.Endpoint{
				{
					Port: blackboxexporter.BlackBoxExporterPortName,
					// Probe every 30s
					Interval: rhobsv1.Duration(ServiceMonitorPeriod),
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*rhobsv1.RelabelConfig{
						{
							Replacement: routeURL,
							TargetLabel: UrlLabelName,
						},
						{
							Replacement: clusterID,
							TargetLabel: "_id",
						},
					},
				}},
			Selector: metav1.LabelSelector{
				MatchLabels: blackboxexporter.GenerateBlackBoxExporterLables(),
			},
			NamespaceSelector: rhobsv1.NamespaceSelector{
				MatchNames: []string{
					blackBoxExporterNamespace,
				},
			},
		},
	}
}
