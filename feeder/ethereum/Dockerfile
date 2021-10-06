FROM node:latest@sha256:7a888d7030be38976daabcd0881ff6564fb05885046fef9d08eb6002fa6516fb

WORKDIR /eth_wit_feeder

# install deps first
COPY yarn.lock package.json ./
RUN yarn install

COPY . .

CMD ["yarn", "start"]
