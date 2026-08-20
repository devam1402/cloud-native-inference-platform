package admission

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := platformv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidateTenantExists(t *testing.T) {
	s := newScheme(t)

	tenant := &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "finance"},
		Spec: platformv1alpha1.TenantSpec{
			ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "1", Memory: "1Gi", GPU: "0"},
			Priority:      platformv1alpha1.TenantPriority{Default: 100},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(tenant).Build()

	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "finance"},
		Spec:       platformv1alpha1.InferenceServiceSpec{Model: "m1", WorkloadClass: "interactive", ResourceProfile: "protected-gpu"},
	}
	if d := validateTenantExists(context.Background(), c, isvc); !d.Allowed {
		t.Errorf("expected allowed, got denied: %s", d.Reason)
	}

	isvcOrphan := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "no-such-tenant"},
		Spec:       platformv1alpha1.InferenceServiceSpec{Model: "m1", WorkloadClass: "interactive", ResourceProfile: "protected-gpu"},
	}
	if d := validateTenantExists(context.Background(), c, isvcOrphan); d.Allowed {
		t.Errorf("expected denied for missing tenant, got allowed")
	} else if d.Code != "TENANT_NOT_FOUND" {
		t.Errorf("expected TENANT_NOT_FOUND, got %s", d.Code)
	}
}

func TestValidateResourceProfile(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		Spec: platformv1alpha1.InferenceServiceSpec{ResourceProfile: "protected-gpu"},
	}
	if d := validateResourceProfile(isvc); !d.Allowed {
		t.Errorf("expected allowed for known profile, got: %s", d.Reason)
	}

	isvcBad := &platformv1alpha1.InferenceService{
		Spec: platformv1alpha1.InferenceServiceSpec{ResourceProfile: "does-not-exist"},
	}
	if d := validateResourceProfile(isvcBad); d.Allowed {
		t.Errorf("expected denied for unknown profile")
	} else if d.Code != "PROFILE_INVALID" {
		t.Errorf("expected PROFILE_INVALID, got %s", d.Code)
	}
}

func TestValidateSLO(t *testing.T) {
	neg := int64(-5)
	isvc := &platformv1alpha1.InferenceService{
		Spec: platformv1alpha1.InferenceServiceSpec{
			SLO: platformv1alpha1.InferenceSLO{TTFTP99Milliseconds: &neg},
		},
	}
	if d := validateSLO(isvc); d.Allowed {
		t.Errorf("expected denied for negative ttftP99")
	}

	pos := int64(500)
	isvcOK := &platformv1alpha1.InferenceService{
		Spec: platformv1alpha1.InferenceServiceSpec{
			SLO: platformv1alpha1.InferenceSLO{TTFTP99Milliseconds: &pos},
		},
	}
	if d := validateSLO(isvcOK); !d.Allowed {
		t.Errorf("expected allowed for positive ttftP99, got: %s", d.Reason)
	}
}
