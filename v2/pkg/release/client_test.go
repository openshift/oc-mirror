package release

import (
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOCPClient(t *testing.T) {
	id := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	updateAPI, err := url.Parse(UpdateURL)
	require.NoError(t, err)
	client, err := NewOCPClient(id)
	require.NoError(t, err)
	expID := id
	expURL := *updateAPI
	actualURL := client.GetURL()
	require.Equal(t, expID, client.GetID())
	require.Equal(t, expURL.String(), actualURL.String())

	// Test parameter settings
	client.SetQueryParams("arch", "channel", "version")
	exp := "arch=arch&channel=channel&id=01234567-0123-0123-0123-0123456789ab&version=version"
	require.Equal(t, exp, client.GetURL().RawQuery)
}

func TestOKDClient(t *testing.T) {
	id := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	updateAPI, err := url.Parse(OkdUpdateURL)
	require.NoError(t, err)
	client, err := NewOKDClient(id)
	require.NoError(t, err)
	expID := id
	expURL := *updateAPI
	actualURL := client.GetURL()
	require.Equal(t, expID, client.GetID())
	require.Equal(t, expURL.String(), actualURL.String())

	// Test parameter settings
	client.SetQueryParams("arch", "channel", "version")
	require.Equal(t, "", client.GetURL().RawQuery)
}

func TestOCPClientWithOvveride(t *testing.T) {
	os.Setenv("UPDATE_URL_OVERRIDE", "localhost.localdomain")
	id := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	//updateAPI, err := url.Parse(UpdateURL)
	//require.NoError(t, err)
	client, err := NewOCPClient(id)
	require.NoError(t, err)
	expID := id
	//expURL := *updateAPI
	actualURL := client.GetURL()
	require.Equal(t, expID, client.GetID())
	require.Equal(t, "localhost.localdomain", actualURL.String())

	// Test parameter settings
	client.SetQueryParams("arch", "channel", "version")
	require.Equal(t, "arch=arch&channel=channel&id=01234567-0123-0123-0123-0123456789ab&version=version", client.GetURL().RawQuery)
}
