package gateway

import (
	"reflect"
	"testing"
)

func TestUniqueStrings(t *testing.T) {
	type args struct {
		addresses [][]string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "Matching",
			args: args{
				addresses: [][]string{{"192.168.0.1", "192.168.0.2"}, {"192.168.0.2", "192.168.0.1"}},
				// serviceIP: []string{"192.168.0.1", "192.168.0.1"},
				// gatewayIP: []string{"192.168.0.2", "192.168.0.1"},
			},
			want: []string{"192.168.0.1", "192.168.0.2"},
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UniqueAddresses(tt.args.addresses...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UniqueStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}
