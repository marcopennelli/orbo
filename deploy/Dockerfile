# Build stage
# Go version must match the version specified in go.mod
FROM golang:1.24.5-bookworm AS builder

# Install OpenCV dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    git \
    pkg-config \
    unzip \
    wget \
    libgtk-3-dev \
    libavcodec-dev \
    libavformat-dev \
    libswscale-dev \
    libv4l-dev \
    libxvidcore-dev \
    libx264-dev \
    libjpeg-dev \
    libpng-dev \
    libtiff-dev \
    gfortran \
    openexr \
    libatlas-base-dev \
    python3-dev \
    python3-numpy \
    libtbb12 \
    libtbb-dev \
    libdc1394-dev \
    libopenexr-dev \
    libgstreamer-plugins-base1.0-dev \
    libgstreamer1.0-dev \
    && rm -rf /var/lib/apt/lists/*

# Install OpenCV
ENV OPENCV_VERSION="4.8.0"
WORKDIR /tmp
RUN wget -O opencv.zip https://github.com/opencv/opencv/archive/${OPENCV_VERSION}.zip && \
    wget -O opencv_contrib.zip https://github.com/opencv/opencv_contrib/archive/${OPENCV_VERSION}.zip && \
    unzip opencv.zip && \
    unzip opencv_contrib.zip && \
    mkdir -p build && cd build

WORKDIR /tmp/build
RUN cmake -D CMAKE_BUILD_TYPE=RELEASE \
    -D CMAKE_INSTALL_PREFIX=/usr/local \
    -D OPENCV_EXTRA_MODULES_PATH=/tmp/opencv_contrib-${OPENCV_VERSION}/modules \
    -D OPENCV_ENABLE_NONFREE=ON \
    -D BUILD_EXAMPLES=OFF \
    -D INSTALL_PYTHON_EXAMPLES=OFF \
    -D INSTALL_C_EXAMPLES=OFF \
    -D BUILD_TESTS=OFF \
    -D BUILD_PERF_TESTS=OFF \
    -D BUILD_opencv_python2=OFF \
    -D BUILD_opencv_python3=OFF \
    -D WITH_V4L=ON \
    -D WITH_GSTREAMER=ON \
    /tmp/opencv-${OPENCV_VERSION}

RUN make -j$(nproc) && make install && ldconfig

# Set PKG_CONFIG_PATH for OpenCV
ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig:$PKG_CONFIG_PATH

# Build application
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o orbo ./cmd/orbo

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies for OpenCV
RUN apt-get update && apt-get install -y \
    libgtk-3-0 \
    libavcodec59 \
    libavformat59 \
    libswscale6 \
    libv4l-0 \
    libxvidcore4 \
    libx264-164 \
    libjpeg62-turbo \
    libpng16-16 \
    libtiff6 \
    libopenexr-3-1-30 \
    libatlas-base-dev \
    libtbb12 \
    libdc1394-25 \
    libgstreamer-plugins-base1.0-0 \
    libgstreamer1.0-0 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy OpenCV libraries from builder
COPY --from=builder /usr/local/lib/libopencv* /usr/local/lib/
COPY --from=builder /usr/local/lib/pkgconfig/opencv4.pc /usr/local/lib/pkgconfig/
COPY --from=builder /usr/local/include/opencv4 /usr/local/include/opencv4
COPY --from=builder /usr/local/share/opencv4 /usr/local/share/opencv4

# Update library cache
RUN ldconfig

# Create non-root user
RUN adduser --disabled-password --gecos "" --uid 1000 appuser

# Create app directory and directories for frames
RUN mkdir -p /app /app/frames && chown -R appuser:appuser /app

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/orbo .

# Ensure binary is executable
RUN chmod +x orbo

# Switch to non-root user
USER appuser

# Expose service port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Run the service
CMD ["./orbo", "--host=0.0.0.0", "--http-port=8080"]