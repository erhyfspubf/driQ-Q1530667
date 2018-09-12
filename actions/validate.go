package actions

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/qri-io/cafs"
	"github.com/qri-io/dataset"
	"github.com/qri-io/dataset/detect"
	"github.com/qri-io/dataset/dsfs"
	"github.com/qri-io/dataset/dsio"
	"github.com/qri-io/dataset/validate"
	"github.com/qri-io/jsonschema"
	"github.com/qri-io/qri/p2p"
	"github.com/qri-io/qri/repo"
)

// Validate checks a dataset body for errors based on a schema
func Validate(node *p2p.QriNode, ref repo.DatasetRef, body, schema cafs.File) (errors []jsonschema.ValError, err error) {
	// TODO: restore validating data from a URL
	// if p.URL != "" && ref.IsEmpty() && o.Schema == nil {
	//   return (lib.NewError(ErrBadArgs, "if you are validating data from a url, please include a dataset name or supply the --schema flag with a file path that Qri can validate against"))
	// }
	if ref.IsEmpty() && body == nil && schema == nil {
		// NewError(ErrBadArgs, "please provide a dataset name, or a supply the --body and --schema flags with file paths")
		err = fmt.Errorf("please provide a dataset name, or a supply the --body and --schema flags with file paths")
		return
	}

	if !ref.IsEmpty() {
		err = repo.CanonicalizeDatasetRef(node.Repo, &ref)
		if err != nil && err != repo.ErrNotFound {
			log.Debug(err.Error())
			err = fmt.Errorf("error with new reference: %s", err.Error())
			return
		}
	}

	var (
		st   = &dataset.Structure{}
		data []byte
	)

	// if a dataset is specified, load it
	if ref.Path != "" {
		if err = DatasetHead(node, &ref); err != nil {
			log.Debug(err.Error())
			return
		}

		ds, e := ref.DecodeDataset()
		if e != nil {
			log.Debug(e.Error())
			err = e
			return
		}

		st = ds.Structure
	} else if body == nil {
		err = fmt.Errorf("cannot find dataset: %s", ref)
		return
	}

	if body != nil {
		data, err = ioutil.ReadAll(body)
		if err != nil {
			log.Debug(err.Error())
			err = fmt.Errorf("error reading data: %s", err.Error())
			return
		}

		// if no schema, detect one
		if st.Schema == nil {
			var df dataset.DataFormat
			df, err = detect.ExtensionDataFormat(body.FileName())
			if err != nil {
				err = fmt.Errorf("detecting data format: %s", err.Error())
				return
			}
			str, _, e := detect.FromReader(df, bytes.NewBuffer(data))
			if e != nil {
				err = fmt.Errorf("error detecting from reader: %s", e)
				return
			}
			st = str
		}
	}

	// if a schema is specified, override with it
	if schema != nil {
		stbytes, e := ioutil.ReadAll(schema)
		if e != nil {
			log.Debug(e.Error())
			err = e
			return
		}
		sch := &jsonschema.RootSchema{}
		if e := sch.UnmarshalJSON(stbytes); e != nil {
			err = fmt.Errorf("error reading schema: %s", e.Error())
			return
		}
		st.Schema = sch
	}

	if data == nil && ref.Dataset != nil {
		ds, e := ref.DecodeDataset()
		if e != nil {
			log.Debug(e.Error())
			err = fmt.Errorf("error loading dataset data: %s", e.Error())
			return
		}

		f, e := dsfs.LoadBody(node.Repo.Store(), ds)
		if e != nil {
			log.Debug(e.Error())
			err = fmt.Errorf("error loading dataset data: %s", e.Error())
			return
		}
		data, err = ioutil.ReadAll(f)
		if err != nil {
			log.Debug(err.Error())
			err = fmt.Errorf("error loading dataset data: %s", err.Error())
			return
		}
	}

	er, err := dsio.NewEntryReader(st, bytes.NewBuffer(data))
	if err != nil {
		log.Debug(err.Error())
		err = fmt.Errorf("error reading data: %s", err.Error())
		return
	}

	return validate.EntryReader(er)
}
