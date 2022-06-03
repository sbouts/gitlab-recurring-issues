package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/adrg/frontmatter"
)

func Test_parsContent(t *testing.T) {
	type args struct {
		contents []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Parses empty content",
			args: args{contents: ([]byte)(`---
title: Test Title
---
`)},
			want: []byte{},
		},
		{
			name: "Parses content",
			args: args{contents: ([]byte)(`---
title: Test Title
---
Test Content
`)},
			want: []byte("Test Content\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mdata metadata
			content, err := frontmatter.Parse(strings.NewReader(string(tt.args.contents)), &mdata)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(content, tt.want) {
				t.Errorf("parseContent() = %v, want %v", string(content), string(tt.want))
			}
		})
	}
}

func Test_parseMetadata(t *testing.T) {
	type args struct {
		contents []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *metadata
		wantErr bool
	}{
		{
			name: "Parses title",
			args: args{contents: ([]byte)(`---
title: Test Title
---
`)},
			want: &metadata{
				Title: "Test Title",
			},
		},
		{
			name: "Parses confidential",
			args: args{contents: ([]byte)(`---
confidential: true
---
`)},
			want: &metadata{
				Confidential: true,
			},
		},
		{
			name: "Parses assignee",
			args: args{contents: ([]byte)(`---
assignees: [ "assignee1" ]
---
`)},
			want: &metadata{
				Assignees: []string{"assignee1"},
			},
		},
		{
			name: "Parses assignees",
			args: args{contents: ([]byte)(`---
assignees: [ "assignee1", "assignee2" ]
---
`)},
			want: &metadata{
				Assignees: []string{"assignee1", "assignee2"},
			},
		},
		{
			name: "Parses label",
			args: args{contents: ([]byte)(`---
labels: [ "label1" ]
---
`)},
			want: &metadata{
				Labels: []string{"label1"},
			},
		},
		{
			name: "Parses labels",
			args: args{contents: ([]byte)(`---
labels: [ "label1", "label2" ]
---
`)},
			want: &metadata{
				Labels: []string{"label1", "label2"},
			},
		},
		{
			name: "Parses dueindays",
			args: args{contents: ([]byte)(`---
duein: 24h
---
`)},
			want: &metadata{
				DueIn: "24h",
			},
		},
		{
			name: "Parses content",
			args: args{contents: ([]byte)(`---
title: Test Title
---
Test Content
`)},
			want: &metadata{
				Title: "Test Title",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mdata metadata
			_, err := frontmatter.Parse(strings.NewReader(string(tt.args.contents)), &mdata)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(&mdata, tt.want) {
				t.Errorf("parseMetadata() = %v, want %v", &mdata, tt.want)
			}
		})
	}
}
