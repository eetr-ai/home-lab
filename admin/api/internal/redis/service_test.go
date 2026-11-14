package redis

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// fakeRepo is a Redis that holds whatever the test put in it. Each method
// records what it was asked, so a test can check the service sent the bounds it
// was supposed to rather than only what came back.
type fakeRepo struct {
	fakeACL

	scanKeys []string
	scanNext uint64
	metas    map[string]KeyMeta
	raw      RawKey
	unlinked int64
	err      error

	gotCursor uint64
	gotMatch  string
	gotCount  int64
	gotItems  int
	gotBytes  int64
}

func (f *fakeRepo) Info(context.Context) (string, error) { return sampleInfo, f.err }

func (f *fakeRepo) Scan(_ context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	f.gotCursor, f.gotMatch, f.gotCount = cursor, match, count
	return f.scanKeys, f.scanNext, f.err
}

func (f *fakeRepo) Describe(_ context.Context, keys []string) ([]KeyMeta, error) {
	ret := make([]KeyMeta, len(keys))
	for i, key := range keys {
		ret[i] = f.metas[key]
	}
	return ret, f.err
}

func (f *fakeRepo) ReadKey(_ context.Context, _ string, items int, bytes int64) (RawKey, error) {
	f.gotItems, f.gotBytes = items, bytes
	return f.raw, f.err
}

func (f *fakeRepo) Unlink(context.Context, string) (int64, error) { return f.unlinked, f.err }

func TestScanKeys(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		cursor    string
		count     int
		repo      fakeRepo
		wantMatch string
		wantCount int64
		wantPage  KeyPage
		wantErr   error
	}{
		{
			name:      "an empty pattern means every key, and zero means the default count",
			repo:      fakeRepo{},
			wantMatch: "*",
			wantCount: defaultScanCount,
			wantPage:  KeyPage{Keys: []KeySummary{}},
		},
		{
			name:      "a count above the ceiling is brought down rather than refused",
			pattern:   "lock:*",
			count:     50000,
			repo:      fakeRepo{},
			wantMatch: "lock:*",
			wantCount: maxScanCount,
			wantPage:  KeyPage{Keys: []KeySummary{}},
		},
		{
			// SCAN's 0 means "finished". Handing it back as a cursor would make the
			// next page start the iteration over, forever.
			name:   "a finished scan hands back no cursor, an unfinished one does",
			cursor: "17",
			repo: fakeRepo{
				scanKeys: []string{"a", "b"},
				scanNext: 42,
				metas: map[string]KeyMeta{
					"a": {Type: "string", TTLMillis: ttlNone},
					"b": {Type: "hash", TTLMillis: 5000},
				},
			},
			wantMatch: "*",
			wantCount: defaultScanCount,
			wantPage: KeyPage{
				NextCursor: "42",
				Keys: []KeySummary{
					{Key: Text{"a", encodingUTF8}, Type: "string", TTLMillis: ttlNone},
					{Key: Text{"b", encodingUTF8}, Type: "hash", TTLMillis: 5000},
				},
			},
		},
		{
			name: "a key that vanished between the scan and the describe is dropped",
			repo: fakeRepo{
				scanKeys: []string{"gone", "expired", "here"},
				metas: map[string]KeyMeta{
					"gone":    {Type: typeNone, TTLMillis: ttlMissing},
					"expired": {Type: "string", TTLMillis: ttlMissing},
					"here":    {Type: "set", TTLMillis: ttlNone},
				},
			},
			wantMatch: "*",
			wantCount: defaultScanCount,
			wantPage: KeyPage{Keys: []KeySummary{
				{Key: Text{"here", encodingUTF8}, Type: "set", TTLMillis: ttlNone},
			}},
		},
		{
			name: "a binary key name comes back base64",
			repo: fakeRepo{
				scanKeys: []string{"\xff\xfe"},
				metas:    map[string]KeyMeta{"\xff\xfe": {Type: "string", TTLMillis: ttlNone}},
			},
			wantMatch: "*",
			wantCount: defaultScanCount,
			wantPage: KeyPage{Keys: []KeySummary{
				{Key: Text{"//4=", encodingBase64}, Type: "string", TTLMillis: ttlNone},
			}},
		},
		{name: "a cursor we did not hand out is refused", cursor: "abc", wantErr: ErrInvalidRequest},
		{name: "a negative count is refused", count: -1, wantErr: ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			page, err := NewService(&repo).ScanKeys(context.Background(), tt.pattern, tt.cursor, tt.count)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ScanKeys() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ScanKeys() unexpected error: %v", err)
			}
			if repo.gotMatch != tt.wantMatch || repo.gotCount != tt.wantCount {
				t.Errorf("SCAN was sent match %q count %d, want %q and %d",
					repo.gotMatch, repo.gotCount, tt.wantMatch, tt.wantCount)
			}
			if !reflect.DeepEqual(page, tt.wantPage) {
				t.Errorf("ScanKeys() =\n%+v\nwant\n%+v", page, tt.wantPage)
			}
		})
	}
}

// items returns n distinct strings, for building collections at and just past
// the bound.
func items(n int) []string {
	ret := make([]string, n)
	for i := range ret {
		ret[i] = fmt.Sprintf("item-%d", i)
	}
	return ret
}

func TestReadKey(t *testing.T) {
	tests := []struct {
		name          string
		raw           RawKey
		wantShown     int
		wantTruncated bool
		wantSupported bool
		wantErr       error
	}{
		{
			name:          "a list holding exactly the bound is not truncated",
			raw:           RawKey{Type: "list", TTLMillis: ttlNone, Length: maxItems, Items: items(maxItems)},
			wantShown:     maxItems,
			wantSupported: true,
		},
		{
			// The repository is asked for one more than we show, and that extra item
			// is the whole signal: it is what makes this "truncated" without asking
			// the server a second question.
			name:          "a list one past the bound shows the bound and says so",
			raw:           RawKey{Type: "list", TTLMillis: ttlNone, Length: 5000, Items: items(maxItems + 1)},
			wantShown:     maxItems,
			wantTruncated: true,
			wantSupported: true,
		},
		{
			name:          "a string longer than the byte bound is truncated",
			raw:           RawKey{Type: "string", TTLMillis: ttlNone, Length: maxStringBytes + 10, String: "abc"},
			wantTruncated: true,
			wantSupported: true,
		},
		{
			name:          "a short string is whole",
			raw:           RawKey{Type: "string", TTLMillis: ttlNone, Length: 3, String: "abc"},
			wantSupported: true,
		},
		{
			name:          "a module's type is reported but not read",
			raw:           RawKey{Type: "ReJSON-RL", TTLMillis: ttlNone, Length: 1},
			wantSupported: false,
		},
		{name: "a missing key is not found", raw: RawKey{Type: typeNone, TTLMillis: ttlMissing}, wantErr: ErrNotFound},
		{
			name:    "a key that expired mid-read is not found",
			raw:     RawKey{Type: "string", TTLMillis: ttlMissing},
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := fakeRepo{raw: tt.raw}
			detail, err := NewService(&repo).ReadKey(context.Background(), "k")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReadKey() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadKey() unexpected error: %v", err)
			}
			if repo.gotItems != maxItems+1 || repo.gotBytes != maxStringBytes {
				t.Errorf("the repository was asked for %d items and %d bytes, want %d and %d",
					repo.gotItems, repo.gotBytes, maxItems+1, maxStringBytes)
			}
			if len(detail.Items) != tt.wantShown {
				t.Errorf("ReadKey() showed %d items, want %d", len(detail.Items), tt.wantShown)
			}
			if detail.Truncated != tt.wantTruncated {
				t.Errorf("ReadKey() truncated = %v, want %v", detail.Truncated, tt.wantTruncated)
			}
			if detail.Supported != tt.wantSupported {
				t.Errorf("ReadKey() supported = %v, want %v", detail.Supported, tt.wantSupported)
			}
		})
	}
}

func TestDeleteKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		removed int64
		wantErr error
	}{
		{name: "a key that was there is removed", key: "lock:abc", removed: 1},
		{name: "a key that was not is not found", key: "lock:abc", removed: 0, wantErr: ErrNotFound},
		{name: "an empty key name is refused before the server sees it", key: "", wantErr: ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewService(&fakeRepo{unlinked: tt.removed}).DeleteKey(context.Background(), tt.key)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("DeleteKey() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
