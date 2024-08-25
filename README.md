![alt text](https://github.com/JaredHane98/AWS-CDK-GO-IMAGE-TRANSFORM/blob/main/Diagram.png?raw=true)


When making a Post request for a URL the user must provide the transforms in a JSON format EG:

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





