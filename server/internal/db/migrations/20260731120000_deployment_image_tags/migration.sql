-- AlterTable
ALTER TABLE "deployments" ADD COLUMN "commit_sha" TEXT;
ALTER TABLE "deployments" ADD COLUMN "image_tags" TEXT;
