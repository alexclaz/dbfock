package encryption

import "testing"

func TestRoundTrip(t *testing.T) {
	service, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := service.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	if cipher == "secret" {
		t.Fatal("value was not encrypted")
	}
	plain, err := service.Decrypt(cipher)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "secret" {
		t.Fatalf("got %q", plain)
	}
}
