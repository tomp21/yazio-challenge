package reconcilers

import (
	"bytes"
	"context"
	"math/rand/v2"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/util"
)

const (
	passwordSpecialChars = "!@#$%^&*()_+-=[]{};':,./?~"
	passwordLetters      = "abcdefghijklmnopqrstuvwxyz"
	passwordNumbers      = "0123456789"
	passwordLength       = 12
)

var PasswordSecretKey = "redis-password"

type SecretReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

func NewSecretReconciler(client client.Client, scheme *runtime.Scheme) *SecretReconciler {
	return &SecretReconciler{
		Client: client,
		scheme: scheme,
	}
}

// Known issue: if the secret was edited and the redis-secret field no longer exist, but the secret itself is there, we are not fixing it
func (r *SecretReconciler) Reconcile(ctx context.Context, redis *cachev1alpha1.Redis) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
			Labels:    util.GetLabels(redis, nil),
		},
	}
	namespacedName := types.NamespacedName{
		Name:      redis.Name,
		Namespace: redis.Namespace,
	}
	password := generateSecureRedisPassword()
	err := r.Get(ctx, namespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			secret.StringData = map[string]string{PasswordSecretKey: password}
			controllerutil.SetControllerReference(redis, secret, r.scheme)
			return r.Create(ctx, secret)
		}
		return err
	}
	return nil
}

// This code is quite ugly, but does the job
func generateSecureRedisPassword() string {
	var password []byte
	//We get an amount of letters that will be uppercase, which at max will be passwordLength/2, then we iterate pulling random letters until we get that amount
	// Using IntN()+1 we ensure the amount is not 0
	upperCaseChars := rand.IntN(passwordLength/2) + 1
	for len(password) < upperCaseChars {
		password = append(password, passwordLetters[rand.IntN(len(passwordLetters)-1)])
	}
	// We set this letters to uppercase and continue pulling at least 1 char of each string of symbols
	password = bytes.ToUpper(password)
	password = append(password, passwordSpecialChars[rand.IntN(len(passwordSpecialChars)-2)]+1)
	password = append(password, passwordLetters[rand.IntN(len(passwordLetters)-2)]+1)
	password = append(password, passwordNumbers[rand.IntN(len(passwordNumbers)-2)]+1)
	allchars := passwordSpecialChars + passwordLetters + passwordNumbers
	// In the end, we keep pulling random characters from a string containing all symbols in order to get the length defined
	for len(password) < passwordLength {
		password = append(password, allchars[rand.IntN(len(allchars)-1)])
	}
	// shuffle them around
	rand.Shuffle(len(password), func(i, j int) {
		password[i], password[j] = password[j], password[i]
	})
	return string(password)
}
