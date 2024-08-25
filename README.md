![alt text](https://github.com/JaredHane98/AWS-CDK-GO-IMAGE-TRANSFORM/blob/main/Diagram.png?raw=true)


The user must provide the image transforms during the POST request

## Example
```json
{
    "ObjectName": "image.jpg",
    "Transforms": [
        {
            "Name": "grayscale"
        },
        {
            "Name": "edgedetection",
            "Params": ["0.5"]
        },
  ]
}
```
## Supported Transforms
-   grayscale
-   sharpen
-   edgedetection
-   dilate
-   erode
-   median
-   emboss
-   invert
-   sepia
-   sharpen
-   sobel


Beware you can very easily max out memory usage of image transform and timeout the lambda. 





