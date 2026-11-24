# EchoDaemon musicnn ONNX exports

These ONNX exports were introduced in EchoDaemon commit `f85eee9`. They are
format conversions of pre-trained [musicnn](https://github.com/jordipons/musicnn)
models, not new training or original EchoDaemon model weights. The archived
TensorFlow checkpoint data for all four models matches the corresponding
upstream Git blobs in tree `516acb2a0ff5ef73f64547898e018e793152c506`.

- `MTT_musicnn.onnx` and `MSD_musicnn.onnx`: musicnn taggers.
- `MTT_vgg.onnx` and `MSD_vgg.onnx`: upstream VGG-style baselines.

Retain the [original ISC notice](LICENSE.musicnn) with any redistribution.
[SHA256SUMS](SHA256SUMS) identifies the exact EchoDaemon ONNX exports.
For an example of model inputs and output labels, see the [genre engine](../server/internal/genreengine)
and [the upstream extractor](https://github.com/jordipons/musicnn/blob/master/musicnn/extractor.py).

**Credit:** Jordi Pons and the Music Technology Group, Universitat Pompeu Fabra.
See the [upstream license](https://github.com/jordipons/musicnn/blob/master/LICENSE.md)
and [musicnn paper](https://arxiv.org/abs/1909.06654).
