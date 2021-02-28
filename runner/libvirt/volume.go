package libvirt

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"

	"libvirt.org/libvirt-go"
	libvirtxml "libvirt.org/libvirt-go-xml"
)

type streamWriter struct {
	stream *libvirt.Stream
}

func (w *streamWriter) Write(p []byte) (int, error) {
	return w.stream.Send(p)
}

func (w *streamWriter) Close() error {
	return w.stream.Finish()
}

var _ io.WriteCloser = &streamWriter{}

func newVolume(name string, size int64) *libvirtxml.StorageVolume {
	return &libvirtxml.StorageVolume{
		Name: name,
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
			Permissions: &libvirtxml.StorageVolumeTargetPermissions{
				// BUG: File mode and group owner of created
				// volumes is hard-coded to 0644 and gid 107,
				// respectively.
				Mode:  "0664",
				Group: "107",
			},
		},
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: uint64(size),
			Unit:  "bytes",
		},
	}
}

func newBackingStoreFromVol(vol *libvirt.StorageVol) (*libvirtxml.StorageVolumeBackingStore, error) {
	path, err := vol.GetPath()
	if err != nil {
		return nil, err
	}

	return &libvirtxml.StorageVolumeBackingStore{
		Path: path,
		Format: &libvirtxml.StorageVolumeTargetFormat{
			// BUG: We only support QCOW2 backing files in QCOW2
			// format. There's no reason we couldn't work with
			// QCOW2 volumes backed by RAW images.
			Type: "qcow2",
		},
	}, nil
}

func createVolumeFromURL(
	ctx context.Context,
	conn *libvirt.Connect,
	pool *libvirt.StoragePool,
	sourceURL string,
) (vol *libvirt.StorageVol, err error) {

	defer func() {
		if err != nil {
			err = fmt.Errorf("createVolumeFromURL: %w", err)
		}
	}()
	u, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("parse-url: %w", err)
	}
	imageName := path.Base(u.Path)

	size, err := fetchImageContentLength(ctx, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("fetch-length: %w", err)
	}

	volXML := newVolume(imageName, size)
	xmlStr, err := volXML.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	vol, err = pool.StorageVolCreateXML(xmlStr, 0)
	if err != nil {
		return nil, fmt.Errorf("vol-create: %w", err)
	}

	stream, err := conn.NewStream(0)
	if err != nil {
		vol.Free()
		return nil, fmt.Errorf("new-stream: %w", err)
	}
	defer stream.Free()

	if err := vol.Upload(stream, 0, uint64(size), 0); err != nil {
		vol.Free()
		stream.Abort()
		return nil, fmt.Errorf("vol-upload: %w", err)
	}

	sw := &streamWriter{stream: stream}
	if err := fetchImage(ctx, sw, sourceURL); err != nil {
		vol.Free()
		stream.Abort()
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if err := stream.Finish(); err != nil {
		vol.Free()
		return nil, fmt.Errorf("stream-finish: %w", err)
	}

	return vol, nil
}
